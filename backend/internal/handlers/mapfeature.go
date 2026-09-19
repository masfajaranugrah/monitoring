package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"monitoring/internal/database"
)

// FeaturePalette dipakai otomatis supaya jalur/area yang dibuat berurutan
// tidak semuanya biru. Admin tetap bisa memilih warna lain per fitur.
var FeaturePalette = []string{
	"#3b82f6", "#ef4444", "#22c55e", "#f59e0b", "#a855f7", "#06b6d4",
	"#f97316", "#84cc16", "#ec4899", "#6366f1", "#14b8a6", "#eab308",
}

var allowedFeatureTypes = map[string]bool{"point": true, "line": true, "polygon": true}
var allowedFeatureIcons = map[string]bool{"dot": true, "customer": true, "wifi": true, "jb": true, "router": true}

func pickPaletteColor(i int) string {
	return FeaturePalette[i%len(FeaturePalette)]
}

func normalizeFeature(in *struct {
	Name        string          `json:"name" binding:"required"`
	FeatureType string          `json:"feature_type" binding:"required"`
	Icon        string          `json:"icon"`
	Color       string          `json:"color"`
	Description json.RawMessage `json:"description"`
	Geometry    json.RawMessage `json:"geometry" binding:"required"`
	Properties  json.RawMessage `json:"properties"`
	Source      string          `json:"source"`
}) (string, error) {
	in.FeatureType = strings.ToLower(strings.TrimSpace(in.FeatureType))
	if !allowedFeatureTypes[in.FeatureType] {
		return "", nil
	}
	if in.Icon == "" || !allowedFeatureIcons[in.Icon] {
		in.Icon = "dot"
	}
	if in.Color == "" {
		in.Color = pickPaletteColor(0)
	}
	in.Color = strings.ToUpper(strings.TrimSpace(in.Color))
	if len(in.Geometry) == 0 || string(in.Geometry) == "null" {
		return "geometry wajib diisi", nil
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	if in.Properties == nil || string(in.Properties) == "null" {
		in.Properties = json.RawMessage(`{}`)
	}
	return "", nil
}

// descriptionString menerima description yang bisa berupa string, object,
// array, atau null dari klien mana pun, lalu menormalkannya jadi teks.
func descriptionString(d json.RawMessage) string {
	if len(d) == 0 || string(d) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(d, &s) == nil {
		return s
	}
	var v interface{}
	if json.Unmarshal(d, &v) == nil && v != nil {
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return ""
}

func ListMapFeatures(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	rows, err := database.Pool.Query(ctx, `
		SELECT id, name, feature_type, icon, color, COALESCE(description, ''),
		       geometry, properties, source, created_by, created_at, updated_at
		FROM map_features
		ORDER BY created_at ASC, id ASC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat fitur peta"})
		return
	}
	defer rows.Close()

	features := make([]map[string]interface{}, 0)
	for rows.Next() {
		var (
			id          int64
			name        string
			featureType string
			icon        string
			color       string
			desc        string
			geometryB   []byte
			propertiesB []byte
			source      string
			createdBy   *int64
			createdAt   time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&id, &name, &featureType, &icon, &color, &desc,
			&geometryB, &propertiesB, &source, &createdBy, &createdAt, &updatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca fitur peta"})
			return
		}
		var geometry json.RawMessage = geometryB
		var properties json.RawMessage = propertiesB
		var gm interface{}
		_ = json.Unmarshal(geometry, &gm)
		var pm interface{}
		if len(properties) > 0 {
			_ = json.Unmarshal(properties, &pm)
		} else {
			pm = map[string]interface{}{}
		}
		features = append(features, map[string]interface{}{
			"id":           id,
			"name":         name,
			"feature_type": featureType,
			"icon":         icon,
			"color":        color,
			"description":  desc,
			"geometry":     gm,
			"properties":   pm,
			"source":       source,
			"created_by":   createdBy,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": features})
}

func CreateMapFeature(c *gin.Context) {
	var in struct {
		Name        string          `json:"name" binding:"required"`
		FeatureType string          `json:"feature_type" binding:"required"`
		Icon        string          `json:"icon"`
		Color       string          `json:"color"`
		Description json.RawMessage `json:"description"`
		Geometry    json.RawMessage `json:"geometry" binding:"required"`
		Properties  json.RawMessage `json:"properties"`
		Source      string          `json:"source"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data fitur tidak lengkap"})
		return
	}
	if msg, _ := normalizeFeature(&in); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	if in.Properties == nil || string(in.Properties) == "null" {
		in.Properties = json.RawMessage(`{}`)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var userID *int64
	if raw, ok := c.Get("user_id"); ok {
		if v, ok := raw.(int64); ok {
			userID = &v
		}
	}

	var (
		id        int64
		createdAt time.Time
		updatedAt time.Time
	)
	err := database.Pool.QueryRow(ctx, `
		INSERT INTO map_features (name, feature_type, icon, color, description, geometry, properties, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		in.Name, in.FeatureType, in.Icon, in.Color, descriptionString(in.Description), string(in.Geometry), string(in.Properties), in.Source, userID,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan fitur peta"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": map[string]interface{}{
		"id":           id,
		"name":         in.Name,
		"feature_type": in.FeatureType,
		"icon":         in.Icon,
		"color":        in.Color,
		"description":  descriptionString(in.Description),
		"geometry":     in.Geometry,
		"properties":   in.Properties,
		"source":       in.Source,
		"created_by":   userID,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	}})
}

func UpdateMapFeature(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var in struct {
		Name        *string          `json:"name"`
		FeatureType *string          `json:"feature_type"`
		Icon        *string          `json:"icon"`
		Color       *string          `json:"color"`
		Description *json.RawMessage `json:"description"`
		Geometry    json.RawMessage  `json:"geometry"`
		Properties  json.RawMessage  `json:"properties"`
		Source      *string          `json:"source"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data fitur tidak valid"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var (
		currentFeatureType string
		currentColor       string
	)
	if err := database.Pool.QueryRow(ctx,
		`SELECT feature_type, color FROM map_features WHERE id = $1`, id,
	).Scan(&currentFeatureType, &currentColor); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "fitur tidak ditemukan"})
		return
	}

	featureType := currentFeatureType
	if in.FeatureType != nil {
		ft := strings.ToLower(strings.TrimSpace(*in.FeatureType))
		if !allowedFeatureTypes[ft] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "feature_type tidak valid"})
			return
		}
		featureType = ft
	}

	icon := "dot"
	color := currentColor
	if in.Icon != nil && *in.Icon != "" && allowedFeatureIcons[*in.Icon] {
		icon = *in.Icon
	}
	if in.Color != nil && strings.TrimSpace(*in.Color) != "" {
		color = strings.ToUpper(strings.TrimSpace(*in.Color))
	}

	newGeometry := in.Geometry
	if len(newGeometry) == 0 {
		newGeometry = nil
	}
	if len(newGeometry) > 0 && (string(newGeometry) == "null") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geometry tidak valid"})
		return
	}

	tag := 1
	args := []interface{}{}
	set := []string{}
	addSet := func(col string, val interface{}) {
		set = append(set, col+" = $"+strconv.Itoa(tag))
		args = append(args, val)
		tag++
	}
	if in.Name != nil {
		addSet("name", *in.Name)
	}
	addSet("feature_type", featureType)
	addSet("icon", icon)
	addSet("color", color)
	if in.Description != nil {
		addSet("description", descriptionString(*in.Description))
	}
	if newGeometry != nil {
		addSet("geometry", string(newGeometry))
	}
	if in.Properties != nil && len(in.Properties) > 0 {
		addSet("properties", string(in.Properties))
	}
	set = append(set, "updated_at = now()")

	query := "UPDATE map_features SET " + strings.Join(set, ", ") + " WHERE id = $" + strconv.Itoa(tag)
	args = append(args, id)
	if _, err := database.Pool.Exec(ctx, query, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memperbarui fitur peta"})
		return
	}

	row := map[string]interface{}{"id": id}
	if in.Name != nil {
		row["name"] = *in.Name
	}
	row["feature_type"] = featureType
	row["icon"] = icon
	row["color"] = color
	if in.Description != nil {
		row["description"] = descriptionString(*in.Description)
	}
	c.JSON(http.StatusOK, gin.H{"data": row})
}

func DeleteMapFeature(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	tag, err := database.Pool.Exec(ctx, `DELETE FROM map_features WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus fitur peta"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "fitur tidak ditemukan"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

// BulkImportMapFeatures dipakai untuk impor hasil Google Earth (KMZ/KML).
// Tiap fitur mendapat warna berbeda dari palet bila warna tidak ditentukan.
func BulkImportMapFeatures(c *gin.Context) {
	var in struct {
		Features []struct {
			Name        string          `json:"name"`
			FeatureType string          `json:"feature_type"`
			Icon        string          `json:"icon"`
			Color       string          `json:"color"`
			Description json.RawMessage `json:"description"`
			Geometry    json.RawMessage `json:"geometry"`
			Properties  json.RawMessage `json:"properties"`
		} `json:"features" binding:"required"`
		Source string `json:"source"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		log.Printf("map feature bulk bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "data impor tidak valid: pastikan body JSON berisi array \"features\" yang tidak kosong",
		})
		return
	}
	if len(in.Features) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tidak ada fitur untuk diimpor"})
		return
	}
	if in.Source != "kmz" && in.Source != "kml" {
		in.Source = "manual"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mulai transaksi"})
		return
	}
	defer tx.Rollback(ctx)

	var userID *int64
	if raw, ok := c.Get("user_id"); ok {
		if v, ok := raw.(int64); ok {
			userID = &v
		}
	}

	created := make([]map[string]interface{}, 0)
	skipped := 0
	for i, feat := range in.Features {
		// Placemark/folder kosong di KMZ/KML tidak punya geometri -> dilewati,
		// supaya satu elemen kosong tidak menggagalkan seluruh impor.
		if len(feat.Geometry) == 0 || string(feat.Geometry) == "null" {
			skipped++
			continue
		}
		var g struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		}
		if err := json.Unmarshal(feat.Geometry, &g); err != nil || g.Type == "" || len(g.Coordinates) == 0 {
			skipped++
			continue
		}
		name := strings.TrimSpace(feat.Name)
		if name == "" {
			name = "Fitur " + strconv.Itoa(i+1)
		}
		ft := strings.ToLower(strings.TrimSpace(feat.FeatureType))
		if !allowedFeatureTypes[ft] {
			ft = "line"
		}
		icon := feat.Icon
		if icon == "" || !allowedFeatureIcons[icon] {
			icon = "dot"
		}
		color := strings.ToUpper(strings.TrimSpace(feat.Color))
		if color == "" {
			color = pickPaletteColor(i)
		}
		props := feat.Properties
		if props == nil || string(props) == "null" {
			props = json.RawMessage(`{}`)
		}

		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO map_features (name, feature_type, icon, color, description, geometry, properties, source, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id`,
			name, ft, icon, color, descriptionString(feat.Description), string(feat.Geometry), string(props), in.Source, userID,
		).Scan(&id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengimpor fitur " + name})
			return
		}
		created = append(created, map[string]interface{}{
			"id":           id,
			"name":         name,
			"feature_type": ft,
			"icon":         icon,
			"color":        color,
			"description":  descriptionString(feat.Description),
			"geometry":     feat.Geometry,
			"source":       in.Source,
		})
	}

	if len(created) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tidak ada fitur valid untuk diimpor"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan impor"})
		return
	}

	resp := gin.H{"data": created}
	if skipped > 0 {
		resp["skipped"] = skipped
	}
	c.JSON(http.StatusCreated, resp)
}
