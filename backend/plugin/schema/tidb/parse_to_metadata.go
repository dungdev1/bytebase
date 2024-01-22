package tidb

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/wrapperspb"

	tidbast "github.com/pingcap/tidb/pkg/parser/ast"
	tidbformat "github.com/pingcap/tidb/pkg/parser/format"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	tidbtypes "github.com/pingcap/tidb/pkg/parser/types"

	tidbparser "github.com/bytebase/bytebase/backend/plugin/parser/tidb"
	"github.com/bytebase/bytebase/backend/plugin/schema"
	storepb "github.com/bytebase/bytebase/proto/generated-go/store"
)

func init() {
	schema.RegisterParseToMetadatas(storepb.Engine_TIDB, ParseToMetadata)
}

const (
	autoIncrementSymbol = "AUTO_INCREMENT"
	autoRandSymbol      = "AUTO_RANDOM"
)

func ParseToMetadata(schema string) (*storepb.DatabaseSchemaMetadata, error) {
	stmts, err := tidbparser.ParseTiDB(schema, "", "")
	if err != nil {
		return nil, err
	}

	transformer := &tidbTransformer{
		state: newDatabaseState(),
	}
	transformer.state.schemas[""] = newSchemaState()

	for _, stmt := range stmts {
		(stmt).Accept(transformer)
	}
	return transformer.state.convertToDatabaseMetadata(), transformer.err
}

type databaseState struct {
	name    string
	schemas map[string]*schemaState
}

func newDatabaseState() *databaseState {
	return &databaseState{
		schemas: make(map[string]*schemaState),
	}
}

func convertToDatabaseState(database *storepb.DatabaseSchemaMetadata) *databaseState {
	state := newDatabaseState()
	state.name = database.Name
	for _, schema := range database.Schemas {
		state.schemas[schema.Name] = convertToSchemaState(schema)
	}
	return state
}

func (s *databaseState) convertToDatabaseMetadata() *storepb.DatabaseSchemaMetadata {
	schemaStates := []*schemaState{}
	for _, schema := range s.schemas {
		schemaStates = append(schemaStates, schema)
	}
	sort.Slice(schemaStates, func(i, j int) bool {
		return schemaStates[i].id < schemaStates[j].id
	})
	schemas := []*storepb.SchemaMetadata{}
	for _, schema := range schemaStates {
		schemas = append(schemas, schema.convertToSchemaMetadata())
	}
	return &storepb.DatabaseSchemaMetadata{
		Name:    s.name,
		Schemas: schemas,
		// Unsupported, for tests only.
		Extensions: []*storepb.ExtensionMetadata{},
	}
}

type schemaState struct {
	id     int
	name   string
	tables map[string]*tableState
}

func newSchemaState() *schemaState {
	return &schemaState{
		tables: make(map[string]*tableState),
	}
}

func convertToSchemaState(schema *storepb.SchemaMetadata) *schemaState {
	state := newSchemaState()
	state.name = schema.Name
	for i, table := range schema.Tables {
		state.tables[table.Name] = convertToTableState(i, table)
	}
	return state
}

func (s *schemaState) convertToSchemaMetadata() *storepb.SchemaMetadata {
	tableStates := []*tableState{}
	for _, table := range s.tables {
		tableStates = append(tableStates, table)
	}
	sort.Slice(tableStates, func(i, j int) bool {
		return tableStates[i].id < tableStates[j].id
	})
	tables := []*storepb.TableMetadata{}
	for _, table := range tableStates {
		tables = append(tables, table.convertToTableMetadata())
	}
	return &storepb.SchemaMetadata{
		Name:   s.name,
		Tables: tables,
		// Unsupported, for tests only.
		Views:             []*storepb.ViewMetadata{},
		Functions:         []*storepb.FunctionMetadata{},
		Streams:           []*storepb.StreamMetadata{},
		Tasks:             []*storepb.TaskMetadata{},
		MaterializedViews: []*storepb.MaterializedViewMetadata{},
	}
}

type tableState struct {
	id          int
	name        string
	columns     map[string]*columnState
	indexes     map[string]*indexState
	foreignKeys map[string]*foreignKeyState
	comment     string
	// Engine and collation are only supported in ParseToMetadata.
	engine    string
	collation string
}

const (
	tableStmtFmt = "" +
		"--\n" +
		"-- Table structure for `%s`\n" +
		"--\n"
)

func (t *tableState) toString(buf *strings.Builder) error {
	if _, err := buf.WriteString(fmt.Sprintf(tableStmtFmt, t.name)); err != nil {
		return err
	}
	if _, err := buf.WriteString(fmt.Sprintf("CREATE TABLE `%s` (\n  ", t.name)); err != nil {
		return err
	}
	columns := []*columnState{}
	for _, column := range t.columns {
		columns = append(columns, column)
	}
	sort.Slice(columns, func(i, j int) bool {
		return columns[i].id < columns[j].id
	})
	for i, column := range columns {
		if i > 0 {
			if _, err := buf.WriteString(",\n  "); err != nil {
				return err
			}
		}
		if err := column.toString(buf); err != nil {
			return err
		}
	}

	indexes := []*indexState{}
	for _, index := range t.indexes {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i].id < indexes[j].id
	})

	for i, index := range indexes {
		if i+len(columns) > 0 {
			if _, err := buf.WriteString(",\n  "); err != nil {
				return err
			}
		}
		if err := index.toString(buf); err != nil {
			return err
		}
	}

	foreignKeys := []*foreignKeyState{}
	for _, fk := range t.foreignKeys {
		foreignKeys = append(foreignKeys, fk)
	}
	sort.Slice(foreignKeys, func(i, j int) bool {
		return foreignKeys[i].id < foreignKeys[j].id
	})

	for i, fk := range foreignKeys {
		if i+len(columns)+len(indexes) > 0 {
			if _, err := buf.WriteString(",\n  "); err != nil {
				return err
			}
		}
		if err := fk.toString(buf); err != nil {
			return err
		}
	}

	if _, err := buf.WriteString("\n)"); err != nil {
		return err
	}

	if t.engine != "" {
		if _, err := buf.WriteString(fmt.Sprintf(" ENGINE=%s", t.engine)); err != nil {
			return err
		}
	}

	if t.collation != "" {
		if _, err := buf.WriteString(fmt.Sprintf(" COLLATE=%s", t.collation)); err != nil {
			return err
		}
	}

	if t.comment != "" {
		if _, err := buf.WriteString(fmt.Sprintf(" COMMENT '%s'", strings.ReplaceAll(t.comment, "'", "''"))); err != nil {
			return err
		}
	}

	if _, err := buf.WriteString(";\n"); err != nil {
		return err
	}
	return nil
}

func newTableState(id int, name string) *tableState {
	return &tableState{
		id:          id,
		name:        name,
		columns:     make(map[string]*columnState),
		indexes:     make(map[string]*indexState),
		foreignKeys: make(map[string]*foreignKeyState),
	}
}

func convertToTableState(id int, table *storepb.TableMetadata) *tableState {
	state := newTableState(id, table.Name)
	state.engine = table.Engine
	state.collation = table.Collation
	state.comment = table.Comment
	for i, column := range table.Columns {
		state.columns[column.Name] = convertToColumnState(i, column)
	}
	for i, index := range table.Indexes {
		state.indexes[index.Name] = convertToIndexState(i, index)
	}
	for i, fk := range table.ForeignKeys {
		state.foreignKeys[fk.Name] = convertToForeignKeyState(i, fk)
	}
	return state
}

func (t *tableState) convertToTableMetadata() *storepb.TableMetadata {
	columnStates := []*columnState{}
	for _, column := range t.columns {
		columnStates = append(columnStates, column)
	}
	sort.Slice(columnStates, func(i, j int) bool {
		return columnStates[i].id < columnStates[j].id
	})
	columns := []*storepb.ColumnMetadata{}
	for _, column := range columnStates {
		columns = append(columns, column.convertToColumnMetadata())
	}
	// Backfill the column positions.
	for i, column := range columns {
		column.Position = int32(i + 1)
	}

	indexStates := []*indexState{}
	for _, index := range t.indexes {
		indexStates = append(indexStates, index)
	}
	sort.Slice(indexStates, func(i, j int) bool {
		return indexStates[i].id < indexStates[j].id
	})
	indexes := []*storepb.IndexMetadata{}
	for _, index := range indexStates {
		indexes = append(indexes, index.convertToIndexMetadata())
	}

	fkStates := []*foreignKeyState{}
	for _, fk := range t.foreignKeys {
		fkStates = append(fkStates, fk)
	}
	sort.Slice(fkStates, func(i, j int) bool {
		return fkStates[i].id < fkStates[j].id
	})
	fks := []*storepb.ForeignKeyMetadata{}
	for _, fk := range fkStates {
		fks = append(fks, fk.convertToForeignKeyMetadata())
	}

	return &storepb.TableMetadata{
		Name:        t.name,
		Columns:     columns,
		Indexes:     indexes,
		ForeignKeys: fks,
		Comment:     t.comment,
		Engine:      t.engine,
		Collation:   t.collation,
	}
}

type foreignKeyState struct {
	id                int
	name              string
	columns           []string
	referencedTable   string
	referencedColumns []string
}

func (f *foreignKeyState) convertToForeignKeyMetadata() *storepb.ForeignKeyMetadata {
	return &storepb.ForeignKeyMetadata{
		Name:              f.name,
		Columns:           f.columns,
		ReferencedTable:   f.referencedTable,
		ReferencedColumns: f.referencedColumns,
	}
}

func convertToForeignKeyState(id int, foreignKey *storepb.ForeignKeyMetadata) *foreignKeyState {
	return &foreignKeyState{
		id:                id,
		name:              foreignKey.Name,
		columns:           foreignKey.Columns,
		referencedTable:   foreignKey.ReferencedTable,
		referencedColumns: foreignKey.ReferencedColumns,
	}
}

func (f *foreignKeyState) toString(buf *strings.Builder) error {
	if _, err := buf.WriteString("CONSTRAINT `"); err != nil {
		return err
	}
	if _, err := buf.WriteString(f.name); err != nil {
		return err
	}
	if _, err := buf.WriteString("` FOREIGN KEY ("); err != nil {
		return err
	}
	for i, column := range f.columns {
		if i > 0 {
			if _, err := buf.WriteString(", "); err != nil {
				return err
			}
		}
		if _, err := buf.WriteString("`"); err != nil {
			return err
		}
		if _, err := buf.WriteString(column); err != nil {
			return err
		}
		if _, err := buf.WriteString("`"); err != nil {
			return err
		}
	}
	if _, err := buf.WriteString(") REFERENCES `"); err != nil {
		return err
	}
	if _, err := buf.WriteString(f.referencedTable); err != nil {
		return err
	}
	if _, err := buf.WriteString("` ("); err != nil {
		return err
	}
	for i, column := range f.referencedColumns {
		if i > 0 {
			if _, err := buf.WriteString(", "); err != nil {
				return err
			}
		}
		if _, err := buf.WriteString("`"); err != nil {
			return err
		}
		if _, err := buf.WriteString(column); err != nil {
			return err
		}
		if _, err := buf.WriteString("`"); err != nil {
			return err
		}
	}
	if _, err := buf.WriteString(")"); err != nil {
		return err
	}
	return nil
}

type indexState struct {
	id      int
	name    string
	keys    []string
	length  []int64
	primary bool
	unique  bool
}

func (i *indexState) convertToIndexMetadata() *storepb.IndexMetadata {
	return &storepb.IndexMetadata{
		Name:        i.name,
		Expressions: i.keys,
		KeyLength:   i.length,
		Primary:     i.primary,
		Unique:      i.unique,
		// Unsupported, for tests only.
		Visible: true,
	}
}

func convertToIndexState(id int, index *storepb.IndexMetadata) *indexState {
	return &indexState{
		id:      id,
		name:    index.Name,
		keys:    index.Expressions,
		length:  index.KeyLength,
		primary: index.Primary,
		unique:  index.Unique,
	}
}

func (i *indexState) toString(buf *strings.Builder) error {
	if i.primary {
		if _, err := buf.WriteString("PRIMARY KEY ("); err != nil {
			return err
		}
		for i, key := range i.keys {
			if i > 0 {
				if _, err := buf.WriteString(", "); err != nil {
					return err
				}
			}
			if _, err := buf.WriteString(fmt.Sprintf("`%s`", key)); err != nil {
				return err
			}
		}
		if _, err := buf.WriteString(")"); err != nil {
			return err
		}
	} else {
		if i.unique {
			if _, err := buf.WriteString("UNIQUE KEY "); err != nil {
				return err
			}
		} else {
			if _, err := buf.WriteString("KEY "); err != nil {
				return err
			}
		}

		if _, err := buf.WriteString(fmt.Sprintf("`%s` (", i.name)); err != nil {
			return err
		}
		for j, key := range i.keys {
			if j > 0 {
				if _, err := buf.WriteString(","); err != nil {
					return err
				}
			}
			if len(key) > 2 && key[0] == '(' && key[len(key)-1] == ')' {
				// Expressions are surrounded by parentheses.
				if _, err := buf.WriteString(key); err != nil {
					return err
				}
			} else {
				columnText := fmt.Sprintf("`%s`", key)
				if len(i.length) > j && i.length[j] > 0 {
					columnText = fmt.Sprintf("`%s`(%d)", key, i.length[j])
				}
				if _, err := buf.WriteString(columnText); err != nil {
					return err
				}
			}
		}
		if _, err := buf.WriteString(")"); err != nil {
			return err
		}
	}
	return nil
}

type defaultValue interface {
	toString() string
}

type defaultValueNull struct {
}

func (*defaultValueNull) toString() string {
	return "NULL"
}

type defaultValueString struct {
	value string
}

func (d *defaultValueString) toString() string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(d.value, "'", "''"))
}

type defaultValueExpression struct {
	value string
}

func (d *defaultValueExpression) toString() string {
	return d.value
}

type columnState struct {
	id   int
	name string
	tp   string
	// hasDefault is true if the column has a default value, auto increment or auto random.
	// These three cases are mutually exclusive.
	hasDefault   bool
	defaultValue defaultValue
	comment      string
	nullable     bool
}

func (c *columnState) hasAutoIncrement() bool {
	return c.hasDefault && strings.EqualFold(c.defaultValue.toString(), autoIncrementSymbol)
}

func (c *columnState) hasAutoRand() bool {
	return c.hasDefault && strings.Contains(strings.ToUpper(c.defaultValue.toString()), autoRandSymbol)
}

func (c *columnState) toString(buf *strings.Builder) error {
	if _, err := buf.WriteString(fmt.Sprintf("`%s` %s", c.name, c.tp)); err != nil {
		return err
	}
	if c.nullable {
		if _, err := buf.WriteString(" NULL"); err != nil {
			return err
		}
	} else {
		if _, err := buf.WriteString(" NOT NULL"); err != nil {
			return err
		}
	}
	if c.hasDefault {
		// todo(zp): refactor column attribute.
		if strings.EqualFold(c.defaultValue.toString(), autoIncrementSymbol) {
			if _, err := buf.WriteString(fmt.Sprintf(" %s", c.defaultValue.toString())); err != nil {
				return err
			}
		} else if strings.Contains(strings.ToUpper(c.defaultValue.toString()), autoRandSymbol) {
			if _, err := buf.WriteString(fmt.Sprintf(" /*T![auto_rand] %s */", c.defaultValue.toString())); err != nil {
				return err
			}
		} else {
			if _, err := buf.WriteString(fmt.Sprintf(" DEFAULT %s", c.defaultValue.toString())); err != nil {
				return err
			}
		}
	}
	if c.comment != "" {
		if _, err := buf.WriteString(fmt.Sprintf(" COMMENT '%s'", c.comment)); err != nil {
			return err
		}
	}
	return nil
}

func (c *columnState) convertToColumnMetadata() *storepb.ColumnMetadata {
	result := &storepb.ColumnMetadata{
		Name:     c.name,
		Type:     c.tp,
		Nullable: c.nullable,
		Comment:  c.comment,
	}
	if c.hasDefault {
		switch value := c.defaultValue.(type) {
		case *defaultValueNull:
			result.DefaultValue = &storepb.ColumnMetadata_DefaultNull{DefaultNull: true}
		case *defaultValueString:
			result.DefaultValue = &storepb.ColumnMetadata_Default{Default: wrapperspb.String(value.value)}
		case *defaultValueExpression:
			result.DefaultValue = &storepb.ColumnMetadata_DefaultExpression{DefaultExpression: value.value}
		}
	}
	return result
}

func convertToColumnState(id int, column *storepb.ColumnMetadata) *columnState {
	result := &columnState{
		id:         id,
		name:       column.Name,
		tp:         column.Type,
		hasDefault: column.GetDefaultValue() != nil,
		nullable:   column.Nullable,
		comment:    column.Comment,
	}
	if result.hasDefault {
		switch value := column.GetDefaultValue().(type) {
		case *storepb.ColumnMetadata_DefaultNull:
			result.defaultValue = &defaultValueNull{}
		case *storepb.ColumnMetadata_Default:
			if value.Default == nil {
				result.defaultValue = &defaultValueNull{}
			} else {
				result.defaultValue = &defaultValueString{value: value.Default.GetValue()}
			}
		case *storepb.ColumnMetadata_DefaultExpression:
			result.defaultValue = &defaultValueExpression{value: value.DefaultExpression}
		}
	}
	return result
}

type tidbTransformer struct {
	tidbast.StmtNode

	state *databaseState
	err   error
}

func (t *tidbTransformer) Enter(in tidbast.Node) (tidbast.Node, bool) {
	if node, ok := in.(*tidbast.CreateTableStmt); ok {
		dbInfo := node.Table.DBInfo
		databaseName := ""
		if dbInfo != nil {
			databaseName = dbInfo.Name.String()
		}
		if databaseName != "" {
			if t.state.name == "" {
				t.state.name = databaseName
			} else if t.state.name != databaseName {
				t.err = errors.New("multiple database names found: " + t.state.name + ", " + databaseName)
				return in, true
			}
		}

		tableName := node.Table.Name.String()
		schema := t.state.schemas[""]
		if _, ok := schema.tables[tableName]; ok {
			t.err = errors.New("multiple table names found: " + tableName)
			return in, true
		}
		schema.tables[tableName] = newTableState(len(schema.tables), tableName)

		table := t.state.schemas[""].tables[tableName]

		// column definition
		for _, column := range node.Cols {
			dataType := columnTypeStr(column.Tp)
			columnName := column.Name.Name.String()
			if _, ok := table.columns[columnName]; ok {
				t.err = errors.New("multiple column names found: " + columnName + " in table " + tableName)
				return in, true
			}

			columnState := &columnState{
				id:       len(table.columns),
				name:     columnName,
				tp:       dataType,
				comment:  "",
				nullable: tidbColumnCanNull(column),
			}

			for _, option := range column.Options {
				switch option.Tp {
				case tidbast.ColumnOptionDefaultValue:
					defaultValue, err := restoreExpr(option.Expr)
					if err != nil {
						t.err = err
						return in, true
					}
					if defaultValue == nil {
						columnState.hasDefault = false
					} else {
						columnState.hasDefault = true
						switch {
						case strings.EqualFold(*defaultValue, "NULL"):
							columnState.defaultValue = &defaultValueNull{}
						case strings.HasPrefix(*defaultValue, "'") && strings.HasSuffix(*defaultValue, "'"):
							columnState.defaultValue = &defaultValueString{value: strings.ReplaceAll((*defaultValue)[1:len(*defaultValue)-1], "''", "'")}
						default:
							columnState.defaultValue = &defaultValueExpression{value: *defaultValue}
						}
					}
				case tidbast.ColumnOptionComment:
					comment, err := restoreComment(option.Expr)
					if err != nil {
						t.err = err
						return in, true
					}
					columnState.comment = comment
				case tidbast.ColumnOptionAutoIncrement:
					defaultValue := autoIncrementSymbol
					columnState.hasDefault = true
					columnState.defaultValue = &defaultValueExpression{value: defaultValue}
				case tidbast.ColumnOptionAutoRandom:
					defaultValue := autoRandSymbol
					unspecifiedLength := -1
					if option.AutoRandOpt.ShardBits != unspecifiedLength {
						if option.AutoRandOpt.RangeBits != unspecifiedLength {
							defaultValue += fmt.Sprintf("(%d, %d)", option.AutoRandOpt.ShardBits, option.AutoRandOpt.RangeBits)
						} else {
							defaultValue += fmt.Sprintf("(%d)", option.AutoRandOpt.ShardBits)
						}
					}
					columnState.hasDefault = true
					columnState.defaultValue = &defaultValueExpression{value: defaultValue}
				}
			}
			table.columns[columnName] = columnState
		}
		for _, tableOption := range node.Options {
			switch tableOption.Tp {
			case tidbast.TableOptionComment:
				table.comment = tableComment(tableOption)
			case tidbast.TableOptionEngine:
				table.engine = tableOption.StrValue
			case tidbast.TableOptionCollate:
				table.collation = tableOption.StrValue
			}
		}

		// primary and foreign key definition
		for _, constraint := range node.Constraints {
			constraintType := constraint.Tp
			switch constraintType {
			case tidbast.ConstraintPrimaryKey:
				var pkList []string
				for _, constraint := range node.Constraints {
					if constraint.Tp == tidbast.ConstraintPrimaryKey {
						var pks []string
						for _, key := range constraint.Keys {
							columnName := key.Column.Name.String()
							pks = append(pks, columnName)
						}
						pkList = append(pkList, pks...)
					}
				}

				table.indexes["PRIMARY"] = &indexState{
					id:      len(table.indexes),
					name:    "PRIMARY",
					keys:    pkList,
					primary: true,
					unique:  true,
				}
			case tidbast.ConstraintForeignKey:
				var referencingColumnList []string
				for _, key := range constraint.Keys {
					referencingColumnList = append(referencingColumnList, key.Column.Name.String())
				}
				var referencedColumnList []string
				for _, spec := range constraint.Refer.IndexPartSpecifications {
					referencedColumnList = append(referencedColumnList, spec.Column.Name.String())
				}

				fkName := constraint.Name
				if fkName == "" {
					t.err = errors.New("empty foreign key name")
					return in, true
				}
				if table.foreignKeys[fkName] != nil {
					t.err = errors.New("multiple foreign keys found: " + fkName)
					return in, true
				}

				fk := &foreignKeyState{
					id:                len(table.foreignKeys),
					name:              fkName,
					columns:           referencingColumnList,
					referencedTable:   constraint.Refer.Table.Name.String(),
					referencedColumns: referencedColumnList,
				}
				table.foreignKeys[fkName] = fk
			case tidbast.ConstraintIndex, tidbast.ConstraintUniq, tidbast.ConstraintUniqKey, tidbast.ConstraintUniqIndex, tidbast.ConstraintKey:
				var referencingColumnList []string
				var lengthList []int64
				for _, spec := range constraint.Keys {
					var specString string
					var err error
					if spec.Column != nil {
						specString = spec.Column.Name.String()
						if spec.Length > 0 {
							lengthList = append(lengthList, int64(spec.Length))
						} else {
							lengthList = append(lengthList, -1)
						}
					} else {
						specString, err = tidbRestoreNode(spec, tidbformat.RestoreKeyWordLowercase|tidbformat.RestoreStringSingleQuotes|tidbformat.RestoreNameBackQuotes)
						if err != nil {
							t.err = err
							return in, true
						}
					}
					referencingColumnList = append(referencingColumnList, specString)
				}

				var indexName string
				if constraint.Name != "" {
					indexName = constraint.Name
				} else {
					t.err = errors.New("empty index name")
					return in, true
				}

				if table.indexes[indexName] != nil {
					t.err = errors.New("multiple foreign keys found: " + indexName)
					return in, true
				}

				table.indexes[indexName] = &indexState{
					id:      len(table.indexes),
					name:    indexName,
					keys:    referencingColumnList,
					length:  lengthList,
					primary: false,
					unique:  constraintType == tidbast.ConstraintUniq || constraintType == tidbast.ConstraintUniqKey || constraintType == tidbast.ConstraintUniqIndex,
				}
			}
		}
	}
	return in, false
}

// columnTypeStr returns the type string of tp.
func columnTypeStr(tp *tidbtypes.FieldType) string {
	// This logic is copy from tidb/pkg/parser/model/model.go:GetTypeDesc()
	// DO NOT TOUCH!
	desc := tp.CompactStr()
	if mysql.HasUnsignedFlag(tp.GetFlag()) && tp.GetType() != mysql.TypeBit && tp.GetType() != mysql.TypeYear {
		desc += " unsigned"
	}
	if mysql.HasZerofillFlag(tp.GetFlag()) && tp.GetType() != mysql.TypeYear {
		desc += " zerofill"
	}
	return desc
}

func tidbColumnCanNull(column *tidbast.ColumnDef) bool {
	for _, option := range column.Options {
		if option.Tp == tidbast.ColumnOptionNotNull || option.Tp == tidbast.ColumnOptionPrimaryKey {
			return false
		}
	}
	return true
}

func restoreExpr(expr tidbast.ExprNode) (*string, error) {
	if expr == nil {
		return nil, nil
	}
	result, err := tidbRestoreNode(expr, tidbformat.RestoreStringSingleQuotes|tidbformat.RestoreStringWithoutCharset)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func tableComment(option *tidbast.TableOption) string {
	return option.StrValue
}

func restoreComment(expr tidbast.ExprNode) (string, error) {
	comment, err := tidbRestoreNode(expr, tidbformat.RestoreStringWithoutCharset)
	if err != nil {
		return "", err
	}
	return comment, nil
}

func equalKeys(a []string, aLength []int64, b []string, bLength []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i, key := range a {
		if key != b[i] {
			return false
		}
		lenA := int64(-1)
		lenB := int64(-1)
		if len(aLength) > i {
			lenA = aLength[i]
		}
		if len(bLength) > i {
			lenB = bLength[i]
		}
		if lenA != lenB {
			return false
		}
	}
	return true
}

func tidbRestoreNode(node tidbast.Node, flag tidbformat.RestoreFlags) (string, error) {
	var buffer strings.Builder
	ctx := tidbformat.NewRestoreCtx(flag, &buffer)
	if err := node.Restore(ctx); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func (*tidbTransformer) Leave(in tidbast.Node) (tidbast.Node, bool) {
	return in, true
}
