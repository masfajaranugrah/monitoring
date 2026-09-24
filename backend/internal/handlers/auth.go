package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"monitoring/internal/config"
	"monitoring/internal/database"
	"monitoring/internal/middleware"
	"monitoring/internal/models"
)

const msQueryTimeout = 5 * time.Second

func ListUsers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	rows, err := database.Pool.Query(ctx,
		`SELECT id, username, full_name, role, is_active, last_login, created_at
		 FROM users ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load users"})
		return
	}
	defer rows.Close()

	users := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var username, fullName, role string
		var isActive bool
		var lastLogin, createdAt time.Time
		if err := rows.Scan(&id, &username, &fullName, &role, &isActive, &lastLogin, &createdAt); err != nil {
			continue
		}
		users = append(users, gin.H{
			"id":         id,
			"username":   username,
			"full_name":  fullName,
			"role":       role,
			"is_active":  isActive,
			"last_login": lastLogin.Format(time.RFC3339),
			"created_at": createdAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": users})
}

func CreateUser(c *gin.Context) {
	var input models.UserCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if input.Role == "" {
		input.Role = models.RoleOperator
	}
	if input.Role != models.RoleAdmin && input.Role != models.RoleOperator {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role must be ADMIN or OPERATOR"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var id int64
	err = database.Pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, full_name, role, is_active)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (username) DO NOTHING
		 RETURNING id`,
		input.Username, string(hash), input.FullName, input.Role, input.IsActive).
		Scan(&id)
	if err == pgx.ErrNoRows {
		c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "User created successfully"})
}

func SetUserActive(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var input struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err = database.Pool.Exec(ctx,
		`UPDATE users SET is_active = $1, updated_at = now() WHERE id = $2`,
		input.IsActive, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User status updated"})
}

func DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Jangan biarkan admin terakhir terhapus sendiri.
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var adminCount int
	_ = database.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE role = 'ADMIN' AND is_active = true`).Scan(&adminCount)
	if adminCount <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete: at least one active ADMIN must remain"})
		return
	}

	_, err = database.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func Login(c *gin.Context) {
	cfg := config.Load()
	var input models.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var user models.User
	err := database.Pool.QueryRow(ctx,
		`SELECT id, username, password_hash, full_name, role, is_active, created_at, updated_at
		 FROM users WHERE username = $1`, input.Username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.FullName,
			&user.Role, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "Account is disabled"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	_, _ = database.Pool.Exec(ctx,
		`UPDATE users SET last_login = now() WHERE id = $1`, user.ID)

	token, err := middleware.GenerateToken(user.ID, user.Username, string(user.Role), cfg.JWTExpiryHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"full_name":  user.FullName,
			"role":       user.Role,
		},
	})
}

func Me(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	c.JSON(http.StatusOK, gin.H{
		"id":         userID,
		"username":   username,
		"role":       role,
	})
}

func ChangePassword(c *gin.Context) {
	var input struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	userID, _ := c.Get("user_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var hash string
	err := database.Pool.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.CurrentPassword)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	_, err = database.Pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		newHash, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}