package modem

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
)

var dmAttrRe = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)

// attrValue mengekstrak nilai atribut dari sebuah tag, mis. name="X".
func attrValue(tag, attr string) string {
	for _, m := range dmAttrRe.FindAllStringSubmatch(tag, -1) {
		if strings.EqualFold(m[1], attr) {
			return html.UnescapeString(m[2])
		}
	}
	return ""
}

// parseDBTables memparse keluaran `sendcmd 1 DB` menjadi tabel-tabel terstruktur.
// Format yang ditangani:
//
//	<Tbl name="DeviceInfo" RowCount="1">
//	  <Row No="0">
//	    <DM name="SerialNumber" val="ZTEG..."/>
//	  </Row>
//	</Tbl>
func parseDBTables(out string) []TableResult {
	var tables []TableResult
	var cur *TableResult
	var curRow Row
	idx := 0
	for idx < len(out) {
		lt := strings.IndexByte(out[idx:], '<')
		if lt < 0 {
			break
		}
		lt += idx
		gt := strings.IndexByte(out[lt:], '>')
		if gt < 0 {
			break
		}
		gt += lt
		tag := out[lt : gt+1]
		idx = gt + 1
		switch {
		case strings.HasPrefix(tag, `<Tbl name="`):
			tables = append(tables, TableResult{Table: attrValue(tag, "name")})
			cur = &tables[len(tables)-1]
			curRow = nil
		case strings.HasPrefix(tag, `<Row No="`):
			if cur != nil {
				curRow = Row{}
				cur.Rows = append(cur.Rows, curRow)
			}
		case strings.HasPrefix(tag, `<DM name="`):
			if curRow != nil {
				curRow[attrValue(tag, "name")] = attrValue(tag, "val")
			}
		}
	}
	return tables
}

// DBGet menjalankan `sendcmd 1 DB get <table>`.
func (t *TelnetClient) DBGet(ctx context.Context, table string) (*TableResult, error) {
	out, err := t.Exec(ctx, "sendcmd 1 DB get "+table)
	if err != nil {
		return nil, err
	}
	tables := parseDBTables(out)
	if len(tables) == 0 {
		return nil, fmt.Errorf("%w: tabel %s kosong (keluaran: %.200s)", ErrNotFound, table, out)
	}
	for i := range tables {
		if strings.EqualFold(tables[i].Table, table) {
			return &tables[i], nil
		}
	}
	res := tables[0]
	return &res, nil
}

// DBGetTable menjalankan `sendcmd 1 DB gettable <table>` (semua instance).
// gettable tidak tersedia di semua firmware; bila gagal, fallback ke DBGet.
func (t *TelnetClient) DBGetTable(ctx context.Context, table string) (*TableResult, error) {
	out, err := t.Exec(ctx, "sendcmd 1 DB gettable "+table)
	if err == nil {
		tables := parseDBTables(out)
		for i := range tables {
			if strings.EqualFold(tables[i].Table, table) {
				return &tables[i], nil
			}
		}
		if len(tables) > 0 {
			return &tables[0], nil
		}
	}
	return t.DBGet(ctx, table)
}

// DBGetIndex menjalankan `sendcmd 1 DB get <table> <index>`.
func (t *TelnetClient) DBGetIndex(ctx context.Context, table string, index int) (Row, error) {
	out, err := t.Exec(ctx, fmt.Sprintf("sendcmd 1 DB get %s %d", table, index))
	if err != nil {
		return nil, err
	}
	tables := parseDBTables(out)
	if len(tables) == 0 || len(tables[0].Rows) == 0 {
		return nil, fmt.Errorf("%w: %s[%d] kosong", ErrNotFound, table, index)
	}
	// Cari baris dengan index yang cocok, jika tidak ada pakai baris pertama.
	for _, r := range tables[0].Rows {
		if r["_idx"] == fmt.Sprintf("%d", index) || r["Index"] == fmt.Sprintf("%d", index) {
			return r, nil
		}
	}
	return tables[0].Rows[0], nil
}

// DBSet menjalankan `sendcmd 1 DB set <table> <index> <field> <value>`.
func (t *TelnetClient) DBSet(ctx context.Context, table string, index int, field, value string) (RawCommandResult, error) {
	cmd := fmt.Sprintf("sendcmd 1 DB set %s %d %s %s", table, index, field, shellQuote(value))
	out, err := t.Exec(ctx, cmd)
	return RawCommandResult{Command: cmd, Output: out}, err
}

// DBSave menyimpan perubahan database ke flash.
func (t *TelnetClient) DBSave(ctx context.Context) (RawCommandResult, error) {
	cmd := "sendcmd 1 DB save"
	out, err := t.Exec(ctx, cmd)
	return RawCommandResult{Command: cmd, Output: out}, err
}

// DBSaveReboot menyimpan lalu reboot perangkat.
func (t *TelnetClient) DBSaveReboot(ctx context.Context) (RawCommandResult, error) {
	cmd := "sendcmd 1 DB save"
	out, err := t.Exec(ctx, cmd)
	return RawCommandResult{Command: cmd, Output: out}, err
}

// Run menjalankan perintah shell bebas (dibatasi oleh pemanggil).
func (t *TelnetClient) Run(ctx context.Context, cmd string) (RawCommandResult, error) {
	out, err := t.Exec(ctx, cmd)
	return RawCommandResult{Command: cmd, Output: out}, err
}

// shellQuote mengapit nilai dengan kutip bila mengandung spasi/karakter khusus.
func shellQuote(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t\"'`+|&;<>()$") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}
