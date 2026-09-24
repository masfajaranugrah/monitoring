package modem

import (
	"context"
	"fmt"
	"strings"
)

// toggleSimple adalah helper generik untuk mengubah field boolean pada tabel
// DB pertama yang cocok, lalu menyimpan konfigurasi.
func (d *Device) toggleSimple(ctx context.Context, section, feature, title string, tables, fields []string) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, name, err := d.FirstTable(ctx, tables...)
	if err != nil {
		return nil, fmt.Errorf("%w: tabel %s tidak ditemukan: %v", ErrNotFound, title, err)
	}
	field := pickField(tbl.Rows[0], fields...)
	if field == "" {
		return nil, fmt.Errorf("%w: field toggle %s tidak dikenali pada %s", ErrUnsupported, title, name)
	}
	// Baca nilai saat ini lalu balikkan.
	cur := strings.TrimSpace(tbl.Rows[0][field])
	newVal := "1"
	if cur == "1" || strings.EqualFold(cur, "true") || strings.EqualFold(cur, "enable") {
		newVal = "0"
	}
	out, err := d.telnet.DBSet(ctx, name, 0, field, newVal)
	if err != nil {
		return nil, err
	}
	save, _ := d.telnet.DBSave(ctx)
	return &Result{
		Section: section, Feature: feature, Title: title,
		Source: "telnet",
		Data: map[string]interface{}{
			"table": name, "field": field, "previous": cur, "value": newVal,
			"set": out, "save": save,
		},
	}, nil
}

// setSimple adalah helper generik untuk mengubah satu field tertentu.
func (d *Device) setSimple(ctx context.Context, section, feature, title, value string,
	tables, fields []string) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, name, err := d.FirstTable(ctx, tables...)
	if err != nil {
		return nil, fmt.Errorf("%w: tabel %s tidak ditemukan: %v", ErrNotFound, title, err)
	}
	field := pickField(tbl.Rows[0], fields...)
	if field == "" {
		return nil, fmt.Errorf("%w: field %s tidak dikenali pada %s", ErrUnsupported, title, name)
	}
	out, err := d.telnet.DBSet(ctx, name, 0, field, value)
	if err != nil {
		return nil, err
	}
	save, _ := d.telnet.DBSave(ctx)
	return &Result{
		Section: section, Feature: feature, Title: title,
		Source: "telnet",
		Data: map[string]interface{}{
			"table": name, "field": field, "value": value, "set": out, "save": save,
		},
	}, nil
}

// containsFold memeriksa substring tanpa memperhatikan besar-kecil huruf.
func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
