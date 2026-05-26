// gen_bson.go
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// bsonKind classifies a field's BSON category.
type bsonKind int

const (
	kindDouble              bsonKind = iota //
	kindString                             //
	kindDocument                           //
	kindArray                              //
	kindBinary                             // []byte, primitive.Binary
	kindObjectID                           // primitive.ObjectID
	kindBoolean                            //
	kindDateTime                           // time.Time
	kindNull                               // primitive.Null
	kindRegex                              // primitive.Regex
	kindJavaScript                         // primitive.JavaScript
	kindJavaScriptWithScope                //
	kindInt32                              //
	kindInt64                              // int64
	kindTimestamp                          // primitive.Timestamp
	kindDecimal128                         // primitive.Decimal128
	kindMinKey                             //
	kindMaxKey                             //
	kindSymbol                             // primitive.Symbol
	kindUndefined                          //
	kindUnknown                            //

	// compound categories
	kindInt               // int
	kindInt8              // int8
	kindInt16             // int16
	kindUint              // uint
	kindUint16            // uint16
	kindUint32            // uint32
	kindUint64            // uint64
	kindFloat32           // float32
	kindPointer           // *T
	kindMap               // map[string]T
	kindStructRef         // struct with //go:bson
	kindPrimitiveD        // primitive.D
	kindPrimitiveA        // primitive.A
	kindPrimitiveM        // primitive.M
	kindByte              // byte
	kindPrimitiveDateTime // primitive.DateTime
	kindAnonStruct        // anonymous struct (struct{...} field)
)

var (
	structMap        = make(map[string]*ast.StructType)
	marshalerTypes   = make(map[string]bool)
	unmarshalerTypes = make(map[string]bool)
	tmpVarSeq        int
)

type fieldInfo struct {
	Name           string
	BsonKey        string
	GoType         string
	Category       bsonKind
	ElemCat        *fieldInfo
	Fields         []fieldInfo // for kindAnonStruct (anonymous struct fields) or non-interface kindStructRef
	OmitEmpty      bool
	MinSize        bool
	Inline         bool
	BinaryIsNative bool   // kindBinary came from []byte (vs primitive.Binary)
	StructName     string // actual struct name for kindStructRef (e.g. "Hero")
}

type structInfo struct {
	Name   string
	Fields []fieldInfo
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: fastbson <file.go|directory>")
	}
	path := os.Args[1]

	fi, err := os.Stat(path)
	if err != nil {
		log.Fatalf("无法访问路径: %v", err)
	}

	fset := token.NewFileSet()

	// Phase 1: Parse all files, build structMap
	type parsedFile struct {
		node *ast.File
		name string
	}
	var parsed []parsedFile

	var files []string
	if fi.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			log.Fatalf("读取目录失败: %v", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") &&
				!strings.HasSuffix(entry.Name(), "_bson.go") &&
				!strings.HasSuffix(entry.Name(), "_unformatted.go") {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
	} else {
		files = append(files, path)
	}
	if len(files) == 0 {
		return
	}

	for _, fileName := range files {
		node, err := parser.ParseFile(fset, fileName, nil, parser.ParseComments)
		if err != nil {
			log.Fatalf("解析文件错误 %s: %v", fileName, err)
		}
		parsed = append(parsed, parsedFile{node, fileName})

		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			hasBson := hasBsonDirective(genDecl)
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if st, ok := typeSpec.Type.(*ast.StructType); ok {
					structMap[typeSpec.Name.Name] = st
					if hasBson {
						marshalerTypes[typeSpec.Name.Name] = true
						unmarshalerTypes[typeSpec.Name.Name] = true
					}
				}
			}
		}

		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
				continue
			}
			typeName := receiverTypeName(funcDecl.Recv.List[0].Type)
			if typeName == "" {
				continue
			}
			if funcDecl.Name.Name == "MarshalBSON" && hasMarshalBSONSignature(funcDecl.Type) {
				marshalerTypes[typeName] = true
			}
			if funcDecl.Name.Name == "UnmarshalBSON" && hasUnmarshalBSONSignature(funcDecl.Type) {
				unmarshalerTypes[typeName] = true
			}
		}
	}

	for _, p := range parsed {
		generateFile(p.node, p.name)
	}
}

func generateFile(node *ast.File, fileName string) {
	var structs []structInfo
	structWhiteList := map[string]bool{}

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		if !hasBsonDirective(genDecl) {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, ok := typeSpec.Type.(*ast.StructType); ok {
				structWhiteList[typeSpec.Name.Name] = true
			}
		}
	}
	if len(structWhiteList) == 0 {
		return
	}

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		if !hasBsonDirective(genDecl) {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			sInfo := structInfo{Name: typeSpec.Name.Name}
			collectFields(structType, token.NewFileSet(), structWhiteList, &sInfo)
			structs = append(structs, sInfo)
		}
	}
	if len(structs) == 0 {
		return
	}

	imps := buildImportList(structs)

	var buf bytes.Buffer
	fmt.Fprintf(&buf,
		"// Code generated by gen_bson.go. DO NOT EDIT.\npackage %s\n\n", node.Name.Name)
	buf.WriteString("import (\n")
	for _, imp := range imps {
		fmt.Fprintf(&buf, "\t%q\n", imp)
	}
	buf.WriteString(")\n\n")

	for _, s := range structs {
		generateMarshal(&buf, &s)
		generateUnmarshal(&buf, &s)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		_ = os.WriteFile(strings.TrimSuffix(fileName, ".go")+"_bson_unformatted.go", buf.Bytes(), 0644)
		log.Fatalf("格式化生成代码失败 %s: %v", fileName, err)
	}

	genFileName := strings.TrimSuffix(fileName, ".go") + "_bson.go"
	if err := os.WriteFile(genFileName, src, 0644); err != nil {
		log.Fatalf("写入生成文件失败: %v", err)
	}
	fmt.Printf("成功生成: %s\n", genFileName)
}
func hasBsonDirective(gd *ast.GenDecl) bool {
	if gd.Doc == nil {
		return false
	}
	for _, c := range gd.Doc.List {
		if strings.TrimSpace(c.Text) == "//go:fastbson" {
			return true
		}
	}
	return false
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func hasMarshalBSONSignature(fn *ast.FuncType) bool {
	if fn.Params != nil && len(fn.Params.List) != 0 {
		return false
	}
	return hasResults(fn, []func(ast.Expr) bool{isByteSliceType, isErrorType})
}

func hasUnmarshalBSONSignature(fn *ast.FuncType) bool {
	if fn.Params == nil || len(fn.Params.List) != 1 {
		return false
	}
	return isByteSliceType(fn.Params.List[0].Type) &&
		hasResults(fn, []func(ast.Expr) bool{isErrorType})
}

func hasResults(fn *ast.FuncType, checks []func(ast.Expr) bool) bool {
	if fn.Results == nil || len(fn.Results.List) != len(checks) {
		return false
	}
	for i, check := range checks {
		if !check(fn.Results.List[i].Type) {
			return false
		}
	}
	return true
}

func isByteSliceType(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	id, ok := arr.Elt.(*ast.Ident)
	return ok && id.Name == "byte"
}

func isErrorType(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "error"
}

func collectFields(st *ast.StructType, fset *token.FileSet, whiteList map[string]bool, out *structInfo) {
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			handleEmbedded(field, fset, whiteList, out)
			continue
		}
		ident := field.Names[0]
		if !ident.IsExported() {
			continue
		}
		fi := parseField(ident.Name, field, fset, whiteList)
		appendFieldOrInline(out, fi, fset, whiteList)
	}
}

func handleEmbedded(field *ast.Field, fset *token.FileSet, whiteList map[string]bool, out *structInfo) {
	var ident *ast.Ident
	if se, ok := field.Type.(*ast.SelectorExpr); ok {
		ident = se.Sel
	} else if id, ok := field.Type.(*ast.Ident); ok {
		ident = id
	} else {
		return
	}
	if !ident.IsExported() {
		return
	}
	fi := parseField(ident.Name, field, fset, whiteList)
	appendFieldOrInline(out, fi, fset, whiteList)
}

func appendFieldOrInline(out *structInfo, fi fieldInfo, fset *token.FileSet, whiteList map[string]bool) {
	if !fi.Inline {
		out.Fields = append(out.Fields, fi)
		return
	}
	expanded := expandInlineFields(fi, fi.Name, fset, whiteList)
	if len(expanded) == 0 {
		out.Fields = append(out.Fields, fi)
		return
	}
	out.Fields = append(out.Fields, expanded...)
}

func expandInlineFields(fi fieldInfo, path string, fset *token.FileSet, whiteList map[string]bool) []fieldInfo {
	switch fi.Category {
	case kindStructRef:
		if len(fi.Fields) == 0 && fi.StructName != "" {
			populateStructFields(&fi, fi.StructName, fset, whiteList)
		}
		var out []fieldInfo
		for _, sf := range fi.Fields {
			sf.Name = path + "." + sf.Name
			if sf.Inline {
				out = append(out, expandInlineFields(sf, sf.Name, fset, whiteList)...)
				continue
			}
			out = append(out, sf)
		}
		return out
	case kindAnonStruct:
		var out []fieldInfo
		for _, sf := range fi.Fields {
			sf.Name = path + "." + sf.Name
			if sf.Inline {
				out = append(out, expandInlineFields(sf, sf.Name, fset, whiteList)...)
				continue
			}
			out = append(out, sf)
		}
		return out
	default:
		return nil
	}
}

func parseField(name string, field *ast.Field, fset *token.FileSet, whiteList map[string]bool) fieldInfo {
	fi := fieldInfo{Name: name}
	fi.BsonKey = strings.ToLower(name)
	fi.parseTag(field)
	fi.resolveCategory(field.Type, fset, whiteList)
	populateGoTypes(&fi, field.Type, fset)

	// Post-processing: for struct refs or pointers/arrays/maps of struct refs that do not implement marshaler/unmarshaler, populate fields.
	var findStructRef func(curr *fieldInfo, visited map[string]bool)
	findStructRef = func(curr *fieldInfo, visited map[string]bool) {
		if curr == nil {
			return
		}
		if curr.Category == kindStructRef && curr.StructName != "" {
			if !marshalerTypes[curr.StructName] || !unmarshalerTypes[curr.StructName] {
				if !visited[curr.StructName] {
					visited[curr.StructName] = true
					if len(curr.Fields) == 0 {
						populateStructFields(curr, curr.StructName, fset, whiteList)
					}
					delete(visited, curr.StructName)
				}
			}
		}
		findStructRef(curr.ElemCat, visited)
		for i := range curr.Fields {
			findStructRef(&curr.Fields[i], visited)
		}
	}
	findStructRef(&fi, make(map[string]bool))

	return fi
}

func populateStructFields(fi *fieldInfo, structName string, fset *token.FileSet, whiteList map[string]bool) {
	st := structMap[structName]
	if st == nil {
		return
	}
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// Embedded field
			var ident *ast.Ident
			if se, ok := field.Type.(*ast.SelectorExpr); ok {
				ident = se.Sel
			} else if id, ok := field.Type.(*ast.Ident); ok {
				ident = id
			}
			if ident != nil && ident.IsExported() {
				sub := parseField(ident.Name, field, fset, whiteList)
				fi.Fields = append(fi.Fields, sub)
			}
			continue
		}
		ident := field.Names[0]
		if !ident.IsExported() {
			continue
		}
		sub := parseField(ident.Name, field, fset, whiteList)
		fi.Fields = append(fi.Fields, sub)
	}
}

func populateGoTypes(fi *fieldInfo, expr ast.Expr, fset *token.FileSet) {
	fi.GoType = typeString(fset, expr)
	switch t := expr.(type) {
	case *ast.StarExpr:
		if fi.ElemCat != nil {
			populateGoTypes(fi.ElemCat, t.X, fset)
		}
	case *ast.ArrayType:
		if fi.ElemCat != nil {
			populateGoTypes(fi.ElemCat, t.Elt, fset)
		}
	case *ast.MapType:
		if fi.ElemCat != nil {
			populateGoTypes(fi.ElemCat, t.Value, fset)
		}
	}
}

func (fi *fieldInfo) parseTag(field *ast.Field) {
	if field.Tag == nil {
		return
	}
	raw := field.Tag.Value
	if !strings.Contains(raw, "bson:") {
		return
	}
	parts := strings.SplitN(raw, "bson:", 2)
	if len(parts) < 2 {
		return
	}
	rest := parts[1]
	start := strings.IndexByte(rest, '"')
	if start < 0 {
		return
	}
	rest = rest[start+1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return
	}
	content := rest[:end]
	if content == "" {
		return
	}
	options := strings.Split(content, ",")
	key := options[0]
	if key == "-" {
		if len(options) > 1 {
			fi.BsonKey = "-"
		} else {
			fi.BsonKey = ""
			return
		}
	} else {
		fi.BsonKey = key
	}
	for _, opt := range options[1:] {
		switch opt {
		case "omitempty":
			fi.OmitEmpty = true
		case "minsize":
			fi.MinSize = true
		case "inline":
			fi.Inline = true
		}
	}
}

func (fi *fieldInfo) resolveCategory(expr ast.Expr, fset *token.FileSet, whiteList map[string]bool) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		fi.Category = kindPointer
		inner := &fieldInfo{}
		inner.resolveCategory(t.X, fset, whiteList)
		fi.ElemCat = inner
	case *ast.ArrayType:
		if id, ok := t.Elt.(*ast.Ident); ok && id.Name == "byte" {
			fi.Category = kindBinary
			fi.BinaryIsNative = true
			return
		}
		fi.Category = kindArray
		inner := &fieldInfo{}
		inner.resolveCategory(t.Elt, fset, whiteList)
		fi.ElemCat = inner
	case *ast.MapType:
		fi.Category = kindMap
		inner := &fieldInfo{}
		inner.resolveCategory(t.Value, fset, whiteList)
		fi.ElemCat = inner
	case *ast.SelectorExpr:
		fi.resolveSelector(t)
	case *ast.StructType:
		fi.Category = kindAnonStruct
		collectAnonFields(t, fset, whiteList, fi)
	case *ast.Ident:
		fi.resolveIdent(t, whiteList)
	default:
		fi.Category = kindUnknown
	}
}

func collectAnonFields(st *ast.StructType, fset *token.FileSet, whiteList map[string]bool, fi *fieldInfo) {
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		ident := field.Names[0]
		if !ident.IsExported() {
			continue
		}
		sub := parseField(ident.Name, field, fset, whiteList)
		fi.Fields = append(fi.Fields, sub)
	}
}

func (fi *fieldInfo) resolveSelector(sel *ast.SelectorExpr) {
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		fi.Category = kindUnknown
		return
	}
	switch pkg.Name + "." + sel.Sel.Name {
	case "time.Time":
		fi.Category = kindDateTime
	case "primitive.DateTime":
		fi.Category = kindPrimitiveDateTime
	case "primitive.ObjectID":
		fi.Category = kindObjectID
	case "primitive.Binary":
		fi.Category = kindBinary
	case "primitive.Regex":
		fi.Category = kindRegex
	case "primitive.Timestamp":
		fi.Category = kindTimestamp
	case "primitive.Decimal128":
		fi.Category = kindDecimal128
	case "primitive.JavaScript":
		fi.Category = kindJavaScript
	case "primitive.Symbol":
		fi.Category = kindSymbol
	case "primitive.Null":
		fi.Category = kindNull
	case "primitive.Undefined":
		fi.Category = kindUndefined
	case "primitive.MinKey":
		fi.Category = kindMinKey
	case "primitive.MaxKey":
		fi.Category = kindMaxKey
	case "primitive.CodeWithScope":
		fi.Category = kindJavaScriptWithScope
	case "primitive.D":
		fi.Category = kindPrimitiveD
	case "primitive.A":
		fi.Category = kindPrimitiveA
	case "primitive.M":
		fi.Category = kindPrimitiveM
	default:
		fi.Category = kindUnknown
	}
}

func (fi *fieldInfo) resolveIdent(id *ast.Ident, whiteList map[string]bool) {
	switch id.Name {
	case "float64":
		fi.Category = kindDouble
	case "float32":
		fi.Category = kindFloat32
	case "string":
		fi.Category = kindString
	case "bool":
		fi.Category = kindBoolean
	case "int":
		fi.Category = kindInt
	case "int8":
		fi.Category = kindInt8
	case "int16":
		fi.Category = kindInt16
	case "int32":
		fi.Category = kindInt32
	case "int64":
		fi.Category = kindInt64
	case "uint":
		fi.Category = kindUint
	case "uint16":
		fi.Category = kindUint16
	case "uint32":
		fi.Category = kindUint32
	case "uint8", "byte":
		fi.Category = kindByte
	case "uint64":
		fi.Category = kindUint64
	default:
		if whiteList[id.Name] || structMap[id.Name] != nil {
			fi.Category = kindStructRef
			fi.StructName = id.Name
		} else {
			fi.Category = kindUnknown
		}
	}
}

func typeString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "unknown"
	}
	return buf.String()
}

func nextTmp(prefix string) string {
	tmpVarSeq++
	return fmt.Sprintf("%s%d", prefix, tmpVarSeq)
}

// ---------------------------------------------------------------------------
// Import list builder
// ---------------------------------------------------------------------------

func buildImportList(structs []structInfo) []string {
	always := map[string]bool{
		"unsafe": true,
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore": true,
	}
	needsFmt := false
	needsStrconv := false
	needsBson := false
	needsPrimitive := false

	var walk func(f *fieldInfo)
	walk = func(f *fieldInfo) {
		switch f.Category {
		case kindArray:
			needsFmt = true
			needsStrconv = true
			walk(f.ElemCat)
		case kindMap:
			needsFmt = true
			walk(f.ElemCat)
		case kindPointer:
			walk(f.ElemCat)
		case kindPrimitiveD, kindPrimitiveA, kindPrimitiveM, kindUnknown:
			needsFmt = true
			needsBson = true
		case kindStructRef:
			needsFmt = true
		case kindUint, kindUint64:
			needsFmt = true
		case kindNull, kindUndefined, kindMinKey, kindMaxKey:
			needsPrimitive = true
		case kindDateTime, kindPrimitiveDateTime, kindRegex, kindTimestamp, kindJavaScript,
			kindJavaScriptWithScope, kindSymbol:
			needsPrimitive = true
		case kindBinary:
			if !f.BinaryIsNative {
				needsPrimitive = true
			}
		}
	}

	for _, s := range structs {
		for _, f := range s.Fields {
			walk(&f)
		}
	}

	var imps []string
	for pkg := range always {
		imps = append(imps, pkg)
	}
	if needsStrconv {
		imps = append(imps, "strconv")
	}
	if needsFmt {
		imps = append(imps, "fmt")
	}
	if needsBson {
		imps = append(imps, "go.mongodb.org/mongo-driver/bson")
	}
	if needsPrimitive {
		imps = append(imps, "go.mongodb.org/mongo-driver/bson/primitive")
	}
	sort.Strings(imps)
	return imps
}

// ---------------------------------------------------------------------------
// Marshal generation
// ---------------------------------------------------------------------------

func generateMarshal(buf *bytes.Buffer, s *structInfo) {
	fmt.Fprintf(buf,
		"func (z *%s) MarshalBSON() ([]byte, error) {\n", s.Name)
	buf.WriteString("\tidx, dst := bsoncore.AppendDocumentStart(nil)\n")

	for _, f := range s.Fields {
		if f.BsonKey == "" {
			continue
		}
		genMarshalField(buf, &f, "\t", "dst", "z")
	}

	buf.WriteString("\tdst, _ = bsoncore.AppendDocumentEnd(dst, idx)\n")
	buf.WriteString("\treturn dst, nil\n}\n\n")
}

func genMarshalField(buf *bytes.Buffer, f *fieldInfo, ind, dstVar, prefix string) {
	ind2 := ind + "\t"

	// Inline fields: expand their sub-fields directly into the parent document.
	if f.Inline {
		if f.Category == kindStructRef {
			// Marshal the inlined struct's fields directly via a temp call.
			// We can't easily inline field-by-field without knowing the sub-struct's
			// field list here; fall back to marshaling the sub-struct and appending
			// its fields one-by-one into the parent document.
			fmt.Fprintf(buf, "%s{\n", ind)
			fmt.Fprintf(buf, "%sinlineBytes, err := %s.%s.MarshalBSON()\n", ind2, prefix, f.Name)
			fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind2)
			fmt.Fprintf(buf, "%sinlineElems, _ := bsoncore.Document(inlineBytes).Elements()\n", ind2)
			fmt.Fprintf(buf, "%sfor _, ie := range inlineElems {\n", ind2)
			fmt.Fprintf(buf, "%s%s = append(%s, ie...)\n", ind2+"\t", dstVar, dstVar)
			fmt.Fprintf(buf, "%s}\n", ind2)
			fmt.Fprintf(buf, "%s}\n", ind)
		}
		// For non-structRef inline (shouldn't normally happen), skip.
		return
	}

	switch f.Category {
	case kindPointer:
		genMarshalPtr(buf, f, ind, dstVar, !f.OmitEmpty, prefix)
		return
	}

	// For omitempty slices/maps/strings: the omit guard (len>0) already implies non-nil,
	// so we don't need a separate nil-null branch inside — just skip the field when empty.
	if f.OmitEmpty {
		writeOmitGuard(buf, f, ind, prefix)
	}

	switch f.Category {
	case kindDouble:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDoubleElement(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindString:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendStringElement(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindBoolean:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendBooleanElement(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindInt32:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt32Element(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindInt64:
		if f.MinSize {
			fmt.Fprintf(buf, "%sif %s.%s >= -2147483648 && %s.%s <= 2147483647 {\n", ind, prefix, f.Name, prefix, f.Name)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt32Element(%s, %q, int32(%s.%s))\n", ind2, dstVar, dstVar, f.BsonKey, prefix, f.Name)
			fmt.Fprintf(buf, "%s} else {\n", ind)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %q, %s.%s)\n", ind2, dstVar, dstVar, f.BsonKey, prefix, f.Name)
			fmt.Fprintf(buf, "%s}\n", ind)
		} else {
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
		}
	case kindInt, kindInt8, kindInt16, kindUint16, kindByte:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt32Element(%s, %q, int32(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindUint:
		fmt.Fprintf(buf, "%sif uint64(%s.%s) > 9223372036854775807 {\n", ind, prefix, f.Name)
		fmt.Fprintf(buf, "%sreturn nil, fmt.Errorf(\"字段 %%s 超出 int64 范围\", %q)\n", ind2, f.BsonKey)
		fmt.Fprintf(buf, "%s}\n", ind)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %q, int64(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindUint32:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %q, int64(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindUint64:
		fmt.Fprintf(buf, "%sif %s.%s > 9223372036854775807 {\n", ind, prefix, f.Name)
		fmt.Fprintf(buf, "%sreturn nil, fmt.Errorf(\"字段 %%s 超出 int64 范围\", %q)\n", ind2, f.BsonKey)
		fmt.Fprintf(buf, "%s}\n", ind)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %q, int64(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindFloat32:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDoubleElement(%s, %q, float64(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindDateTime:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDateTimeElement(%s, %q, %s.%s.UnixMilli())\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDateTimeElement(%s, %q, int64(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindObjectID:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendObjectIDElement(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindBinary:
		if f.BinaryIsNative {
			fmt.Fprintf(buf, "%sif %s.%s == nil {\n", ind, prefix, f.Name)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendNullElement(%s, %q)\n", ind+"	", dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s} else {\n", ind)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendBinaryElement(%s, %q, 0, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
			fmt.Fprintf(buf, "%s}\n", ind)
		} else {
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendBinaryElement(%s, %q, %s.%s.Subtype, %s.%s.Data)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name, prefix, f.Name)
		}
	case kindRegex:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendRegexElement(%s, %q, %s.%s.Pattern, %s.%s.Options)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name, prefix, f.Name)
	case kindTimestamp:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendTimestampElement(%s, %q, %s.%s.T, %s.%s.I)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name, prefix, f.Name)
	case kindDecimal128:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDecimal128Element(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindJavaScript:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendJavaScriptElement(%s, %q, string(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindSymbol:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendSymbolElement(%s, %q, string(%s.%s))\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)
	case kindNull:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendNullElement(%s, %q)\n", ind, dstVar, dstVar, f.BsonKey)
	case kindUndefined:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendUndefinedElement(%s, %q)\n", ind, dstVar, dstVar, f.BsonKey)
	case kindMinKey:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendMinKeyElement(%s, %q)\n", ind, dstVar, dstVar, f.BsonKey)
	case kindMaxKey:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendMaxKeyElement(%s, %q)\n", ind, dstVar, dstVar, f.BsonKey)
	case kindJavaScriptWithScope:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendCodeWithScopeElement(%s, %q, %s.%s)\n", ind, dstVar, dstVar, f.BsonKey, prefix, f.Name)

	case kindArray:
		if f.OmitEmpty {
			// omitempty: len>0 guard already emitted above — just generate the array directly.
			fmt.Fprintf(buf, "%saIdx, aDst := bsoncore.AppendArrayStart(nil)\n", ind)
			fmt.Fprintf(buf, "%sfor i, v := range %s.%s {\n", ind, prefix, f.Name)
			fmt.Fprintf(buf, "%skey := strconv.Itoa(i)\n", ind+"\t")
			genMarshalValue(buf, f.ElemCat, "key", ind+"\t", "v", "aDst")
			fmt.Fprintf(buf, "%s}\n", ind)
			fmt.Fprintf(buf, "%saDst, _ = bsoncore.AppendArrayEnd(aDst, aIdx)\n", ind)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendArrayElement(%s, %q, aDst)\n", ind, dstVar, dstVar, f.BsonKey)
		} else {
			fmt.Fprintf(buf, "%sif %s.%s == nil {\n", ind, prefix, f.Name)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendNullElement(%s, %q)\n", ind2, dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s} else {\n", ind)
			fmt.Fprintf(buf, "%saIdx, aDst := bsoncore.AppendArrayStart(nil)\n", ind2)
			fmt.Fprintf(buf, "%sfor i, v := range %s.%s {\n", ind2, prefix, f.Name)
			fmt.Fprintf(buf, "%skey := strconv.Itoa(i)\n", ind2+"\t")
			genMarshalValue(buf, f.ElemCat, "key", ind2+"\t", "v", "aDst")
			fmt.Fprintf(buf, "%s}\n", ind2)
			fmt.Fprintf(buf, "%saDst, _ = bsoncore.AppendArrayEnd(aDst, aIdx)\n", ind2)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendArrayElement(%s, %q, aDst)\n", ind2, dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s}\n", ind)
		}

	case kindMap:
		if f.OmitEmpty {
			// omitempty: len>0 guard already emitted above — just generate the map directly.
			fmt.Fprintf(buf, "%smIdx, mDst := bsoncore.AppendDocumentStart(nil)\n", ind)
			fmt.Fprintf(buf, "%sfor k, v := range %s.%s {\n", ind, prefix, f.Name)
			genMarshalValue(buf, f.ElemCat, "k", ind+"\t", "v", "mDst")
			fmt.Fprintf(buf, "%s}\n", ind)
			fmt.Fprintf(buf, "%smDst, _ = bsoncore.AppendDocumentEnd(mDst, mIdx)\n", ind)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, mDst)\n", ind, dstVar, dstVar, f.BsonKey)
		} else {
			fmt.Fprintf(buf, "%sif %s.%s == nil {\n", ind, prefix, f.Name)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendNullElement(%s, %q)\n", ind2, dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s} else {\n", ind)
			fmt.Fprintf(buf, "%smIdx, mDst := bsoncore.AppendDocumentStart(nil)\n", ind2)
			fmt.Fprintf(buf, "%sfor k, v := range %s.%s {\n", ind2, prefix, f.Name)
			genMarshalValue(buf, f.ElemCat, "k", ind2+"\t", "v", "mDst")
			fmt.Fprintf(buf, "%s}\n", ind2)
			fmt.Fprintf(buf, "%smDst, _ = bsoncore.AppendDocumentEnd(mDst, mIdx)\n", ind2)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, mDst)\n", ind2, dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s}\n", ind)
		}

	case kindPrimitiveD, kindPrimitiveM:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, err := bson.Marshal(%s.%s)\n", ind2, prefix, f.Name)
		fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind2)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, subBytes)\n", ind2, dstVar, dstVar, f.BsonKey)
		fmt.Fprintf(buf, "%s}\n", ind)
	case kindPrimitiveA:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, err := bson.Marshal(%s.%s)\n", ind2, prefix, f.Name)
		fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind2)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendArrayElement(%s, %q, subBytes)\n", ind2, dstVar, dstVar, f.BsonKey)
		fmt.Fprintf(buf, "%s}\n", ind)

	case kindStructRef:
		if marshalerTypes[f.StructName] {
			fmt.Fprintf(buf, "%s{\n", ind)
			fmt.Fprintf(buf, "%ssubBytes, err := %s.%s.MarshalBSON()\n", ind2, prefix, f.Name)
			fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind2)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, subBytes)\n", ind2, dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s}\n", ind)
		} else {
			// Inline marshalling of structref fields
			fmt.Fprintf(buf, "%s{\n", ind)
			fmt.Fprintf(buf, "%saj, ajDst := bsoncore.AppendDocumentStart(nil)\n", ind2)
			for _, sf := range f.Fields {
				sfCopy := sf
				sfCopy.Name = f.Name + "." + sf.Name
				genMarshalField(buf, &sfCopy, ind2, "ajDst", prefix)
			}
			fmt.Fprintf(buf, "%sajDst, _ = bsoncore.AppendDocumentEnd(ajDst, aj)\n", ind2)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, ajDst)\n", ind2, dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s}\n", ind)
		}

	case kindAnonStruct:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%saj, ajDst := bsoncore.AppendDocumentStart(nil)\n", ind2)
		for _, sf := range f.Fields {
			sfCopy := sf
			sfCopy.Name = f.Name + "." + sf.Name
			genMarshalField(buf, &sfCopy, ind2, "ajDst", prefix)
		}
		fmt.Fprintf(buf, "%sajDst, _ = bsoncore.AppendDocumentEnd(ajDst, aj)\n", ind2)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, ajDst)\n", ind2, dstVar, dstVar, f.BsonKey)
		fmt.Fprintf(buf, "%s}\n", ind)

	default:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, err := bson.Marshal(%s.%s)\n", ind2, prefix, f.Name)
		fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind2)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, subBytes)\n", ind2, dstVar, dstVar, f.BsonKey)
		fmt.Fprintf(buf, "%s}\n", ind)
	}

	// Close the omitEmpty guard.
	if f.OmitEmpty {
		fmt.Fprintf(buf, "%s}\n", ind)
	}
}

func writeOmitGuard(buf *bytes.Buffer, f *fieldInfo, ind, prefix string) {
	switch f.Category {
	case kindString, kindArray, kindMap, kindBinary:
		fmt.Fprintf(buf, "%sif len(%s.%s) > 0 {\n", ind, prefix, f.Name)
	case kindInt, kindInt8, kindInt16, kindInt32, kindInt64, kindUint, kindUint16, kindUint32, kindUint64, kindByte, kindFloat32, kindDouble:
		fmt.Fprintf(buf, "%sif %s.%s != 0 {\n", ind, prefix, f.Name)
	case kindBoolean:
		fmt.Fprintf(buf, "%sif %s.%s {\n", ind, prefix, f.Name)
	case kindPointer:
		// handled by genMarshalPtr
	case kindDateTime:
		fmt.Fprintf(buf, "%sif !%s.%s.IsZero() {\n", ind, prefix, f.Name)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%sif %s.%s != 0 {\n", ind, prefix, f.Name)
	case kindStructRef:
		fmt.Fprintf(buf, "%sif true {\n", ind)
	default:
		fmt.Fprintf(buf, "%sif true {\n", ind)
	}
}

func genMarshalPtr(buf *bytes.Buffer, f *fieldInfo, ind, dstVar string, hasNullElse bool, prefix string) {
	ind2 := ind + "\t"
	elem := f.ElemCat

	fmt.Fprintf(buf, "%sif %s.%s != nil {\n", ind, prefix, f.Name)

	if elem.Category == kindStructRef {
		if marshalerTypes[elem.StructName] {
			fmt.Fprintf(buf, "%ssubBytes, err := %s.%s.MarshalBSON()\n", ind2, prefix, f.Name)
			fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind2)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, subBytes)\n", ind2, dstVar, dstVar, f.BsonKey)
		} else {
			// Inline marshalling of pointer-to-struct
			fmt.Fprintf(buf, "%s{\n", ind2)
			fmt.Fprintf(buf, "%saj, ajDst := bsoncore.AppendDocumentStart(nil)\n", ind2+"\t")
			for _, sf := range elem.Fields {
				sfCopy := sf
				sfCopy.Name = f.Name + "." + sf.Name
				genMarshalField(buf, &sfCopy, ind2+"\t", "ajDst", prefix)
			}
			fmt.Fprintf(buf, "%sajDst, _ = bsoncore.AppendDocumentEnd(ajDst, aj)\n", ind2+"\t")
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %q, ajDst)\n", ind2+"\t", dstVar, dstVar, f.BsonKey)
			fmt.Fprintf(buf, "%s}\n", ind2)
		}
	} else {
		genMarshalValue(buf, elem, strconv.Quote(f.BsonKey), ind2, "*"+prefix+"."+f.Name, dstVar)
	}

	if hasNullElse {
		// Bug fix: use ind (not ind2) so "} else {" is at the same level as "if z.X != nil {"
		fmt.Fprintf(buf, "%s} else {\n", ind)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendNullElement(%s, %q)\n", ind2, dstVar, dstVar, f.BsonKey)
	}
	fmt.Fprintf(buf, "%s}\n", ind)
}

// genMarshalValue generates code for a loop variable value.
func genMarshalValue(buf *bytes.Buffer, f *fieldInfo, key string, ind, val, dstVar string) {
	switch f.Category {
	case kindDouble:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDoubleElement(%s, %s, %s)\n", ind, dstVar, dstVar, key, val)
	case kindString:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendStringElement(%s, %s, %s)\n", ind, dstVar, dstVar, key, val)
	case kindBoolean:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendBooleanElement(%s, %s, %s)\n", ind, dstVar, dstVar, key, val)
	case kindInt32:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt32Element(%s, %s, %s)\n", ind, dstVar, dstVar, key, val)
	case kindInt64:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %s, %s)\n", ind, dstVar, dstVar, key, val)
	case kindInt, kindInt8, kindInt16, kindUint16, kindByte:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt32Element(%s, %s, int32(%s))\n", ind, dstVar, dstVar, key, val)
	case kindUint:
		fmt.Fprintf(buf, "%sif uint64(%s) > 9223372036854775807 {\n", ind, val)
		fmt.Fprintf(buf, "%sreturn nil, fmt.Errorf(\"字段 %%s 超出 int64 范围\", %s)\n", ind+"\t", key)
		fmt.Fprintf(buf, "%s}\n", ind)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %s, int64(%s))\n", ind, dstVar, dstVar, key, val)
	case kindUint32:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %s, int64(%s))\n", ind, dstVar, dstVar, key, val)
	case kindUint64:
		fmt.Fprintf(buf, "%sif %s > 9223372036854775807 {\n", ind, val)
		fmt.Fprintf(buf, "%sreturn nil, fmt.Errorf(\"字段 %%s 超出 int64 范围\", %s)\n", ind+"\t", key)
		fmt.Fprintf(buf, "%s}\n", ind)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendInt64Element(%s, %s, int64(%s))\n", ind, dstVar, dstVar, key, val)
	case kindFloat32:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDoubleElement(%s, %s, float64(%s))\n", ind, dstVar, dstVar, key, val)
	case kindDateTime:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDateTimeElement(%s, %s, %s.UnixMilli())\n", ind, dstVar, dstVar, key, val)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDateTimeElement(%s, %s, int64(%s))\n", ind, dstVar, dstVar, key, val)
	case kindObjectID:
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendObjectIDElement(%s, %s, %s)\n", ind, dstVar, dstVar, key, val)
	case kindArray:
		// Bug fix: use a uniquely-named inner dst variable to avoid shadowing the outer aDst.
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%sinnerIdx, innerDst := bsoncore.AppendArrayStart(nil)\n", ind+"	")
		fmt.Fprintf(buf, "%sfor i, v := range %s {\n", ind+"	", val)
		fmt.Fprintf(buf, "%sk := strconv.Itoa(i)\n", ind+"		")
		genMarshalValue(buf, f.ElemCat, "k", ind+"		", "v", "innerDst")
		fmt.Fprintf(buf, "%s}\n", ind+"	")
		fmt.Fprintf(buf, "%sinnerDst, _ = bsoncore.AppendArrayEnd(innerDst, innerIdx)\n", ind+"	")
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendArrayElement(%s, %s, innerDst)\n", ind+"	", dstVar, dstVar, key)
		fmt.Fprintf(buf, "%s}\n", ind)
	case kindBinary:
		if f.BinaryIsNative {
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendBinaryElement(%s, %s, 0, %s)\n", ind, dstVar, dstVar, key, val)
		} else {
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendBinaryElement(%s, %s, %s.Subtype, %s.Data)\n", ind, dstVar, dstVar, key, val, val)
		}
	case kindStructRef:
		fmt.Fprintf(buf, "%s{\n", ind)
		if marshalerTypes[f.StructName] {
			fmt.Fprintf(buf, "%ssubBytes, err := (%s).MarshalBSON()\n", ind, val)
			fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %s, subBytes)\n", ind, dstVar, dstVar, key)
		} else {
			fmt.Fprintf(buf, "%ssubIdx, subDst := bsoncore.AppendDocumentStart(nil)\n", ind)
			for _, sf := range f.Fields {
				genMarshalField(buf, &sf, ind, "subDst", val)
			}
			fmt.Fprintf(buf, "%ssubDst, _ = bsoncore.AppendDocumentEnd(subDst, subIdx)\n", ind)
			fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %s, subDst)\n", ind, dstVar, dstVar, key)
		}
		fmt.Fprintf(buf, "%s}\n", ind)
	case kindPointer:
		fmt.Fprintf(buf, "%sif %s != nil {\n", ind, val)
		genMarshalValue(buf, f.ElemCat, key, ind+"\t", "(*"+val+")", dstVar)
		fmt.Fprintf(buf, "%s} else {\n", ind+"\t")
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendNullElement(%s, %s)\n", ind+"\t", dstVar, dstVar, key)
		fmt.Fprintf(buf, "%s}\n", ind)
	default:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, err := bson.Marshal(%s)\n", ind, val)
		fmt.Fprintf(buf, "%sif err != nil { return nil, err }\n", ind)
		fmt.Fprintf(buf, "%s%s = bsoncore.AppendDocumentElement(%s, %s, subBytes)\n", ind, dstVar, dstVar, key)
		fmt.Fprintf(buf, "%s}\n", ind)
	}
}

// ---------------------------------------------------------------------------
// Unmarshal generation
// ---------------------------------------------------------------------------

func generateUnmarshal(buf *bytes.Buffer, s *structInfo) {
	fmt.Fprintf(buf,
		"func (z *%s) UnmarshalBSON(b []byte) error {\n", s.Name)
	buf.WriteString("\t_, data, ok := bsoncore.ReadLength(b)\n")
	buf.WriteString("\tif !ok {\n")
	buf.WriteString("\t\treturn fmt.Errorf(\"invalid BSON document\")\n")
	buf.WriteString("\t}\n")
	buf.WriteString("\tfor len(data) > 0 && data[0] != 0 {\n")
	buf.WriteString("\t\telem, rem, ok := bsoncore.ReadElement(data)\n")
	buf.WriteString("\t\tif !ok {\n")
	buf.WriteString("\t\t\treturn fmt.Errorf(\"invalid BSON element\")\n")
	buf.WriteString("\t\t}\n")
	buf.WriteString("\t\tdata = rem\n")
	buf.WriteString("\n")
	buf.WriteString("\t\tval := elem.Value()\n")
	buf.WriteString("\n")
	buf.WriteString("\t\tkeyBytes := elem.KeyBytes()\n")

	// Generate the key dispatch table
	genKeyDispatch(buf, s.Fields, "\t\t")
	buf.WriteString("\t}\n")
	buf.WriteString("\treturn nil\n")
	buf.WriteString("}\n\n")
}

func genKeyDispatch(buf *bytes.Buffer, fields []fieldInfo, ind string) {
	fmt.Fprintf(buf, "%sswitch unsafe.String(unsafe.SliceData(keyBytes), len(keyBytes)) {\n", ind)
	for _, f := range fields {
		if f.BsonKey == "" {
			continue
		}
		fmt.Fprintf(buf, "%scase %q:\n", ind+"\t", f.BsonKey)
		genUnmarshalCaseBody(buf, &f, ind+"\t\t", "val")
	}
	fmt.Fprintf(buf, "%s}\n", ind)
}


func genUnmarshalCaseBody(buf *bytes.Buffer, f *fieldInfo, ind2 string, valExpr string) {
	ind3 := ind2 + "\t"

	switch f.Category {
	case kindDouble:
		fmt.Fprintf(buf, "%sz.%s = %s.Double()\n", ind2, f.Name, valExpr)

	case kindString:
		fmt.Fprintf(buf, "%sz.%s = %s.StringValue()\n", ind2, f.Name, valExpr)

	case kindBoolean:
		fmt.Fprintf(buf, "%sz.%s = %s.Boolean()\n", ind2, f.Name, valExpr)

	case kindInt32:
		fmt.Fprintf(buf, "%sz.%s = %s.Int32()\n", ind2, f.Name, valExpr)

	case kindInt64:
		fmt.Fprintf(buf, "%sz.%s = %s.AsInt64()\n", ind2, f.Name, valExpr)

	case kindInt:
		fmt.Fprintf(buf, "%sz.%s = int(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindInt8:
		fmt.Fprintf(buf, "%sz.%s = int8(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindInt16:
		fmt.Fprintf(buf, "%sz.%s = int16(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindUint:
		fmt.Fprintf(buf, "%sz.%s = uint(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindUint16:
		fmt.Fprintf(buf, "%sz.%s = uint16(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindUint32:
		fmt.Fprintf(buf, "%sz.%s = uint32(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindUint64:
		fmt.Fprintf(buf, "%sz.%s = uint64(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindByte:
		fmt.Fprintf(buf, "%sz.%s = byte(%s.AsInt64())\n", ind2, f.Name, valExpr)

	case kindFloat32:
		fmt.Fprintf(buf, "%sz.%s = float32(%s.Double())\n", ind2, f.Name, valExpr)

	case kindDateTime:
		fmt.Fprintf(buf, "%sz.%s = primitive.DateTime(%s.DateTime()).Time()\n", ind2, f.Name, valExpr)

	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%sz.%s = primitive.DateTime(%s.DateTime())\n", ind2, f.Name, valExpr)

	case kindObjectID:
		fmt.Fprintf(buf, "%sz.%s = %s.ObjectID()\n", ind2, f.Name, valExpr)

	case kindBinary:
		if f.BinaryIsNative {
			fmt.Fprintf(buf, "%sif %s.Type == 0x0A {\n", ind2, valExpr)

			fmt.Fprintf(buf, "%sz.%s = nil\n", ind3, f.Name)
			fmt.Fprintf(buf, "%s} else {\n", ind2)
			fmt.Fprintf(buf, "%s_, z.%s, _ = %s.BinaryOK()\n", ind3, f.Name, valExpr)

			fmt.Fprintf(buf, "%s}\n", ind2)
		} else {
			fmt.Fprintf(buf, "%ssubtype, data, _ := %s.BinaryOK(); z.%s = primitive.Binary{Subtype: subtype, Data: data}\n", ind2, valExpr, f.Name)

		}
	case kindRegex:
		fmt.Fprintf(buf, "%spattern, options := %s.Regex(); z.%s = primitive.Regex{Pattern: pattern, Options: options}\n", ind2, valExpr, f.Name)

	case kindTimestamp:
		fmt.Fprintf(buf, "%st, i := %s.Timestamp(); z.%s = primitive.Timestamp{T: t, I: i}\n", ind2, valExpr, f.Name)

	case kindDecimal128:
		fmt.Fprintf(buf, "%sz.%s = %s.Decimal128()\n", ind2, f.Name, valExpr)

	case kindJavaScript:
		fmt.Fprintf(buf, "%sz.%s = primitive.JavaScript(%s.JavaScript())\n", ind2, f.Name, valExpr)

	case kindSymbol:
		fmt.Fprintf(buf, "%sz.%s = primitive.Symbol(%s.Symbol())\n", ind2, f.Name, valExpr)

	case kindNull:
		fmt.Fprintf(buf, "%sz.%s = primitive.Null{}\n", ind2, f.Name)
	case kindUndefined:
		fmt.Fprintf(buf, "%sz.%s = primitive.Undefined{}\n", ind2, f.Name)
	case kindMinKey:
		fmt.Fprintf(buf, "%sz.%s = primitive.MinKey{}\n", ind2, f.Name)
	case kindMaxKey:
		fmt.Fprintf(buf, "%sz.%s = primitive.MaxKey{}\n", ind2, f.Name)
	case kindJavaScriptWithScope:
		fmt.Fprintf(buf, "%scode, scope := %s.CodeWithScope(); z.%s = primitive.CodeWithScope{Code: primitive.JavaScript(code), Scope: scope}\n", ind2, valExpr, f.Name)

	case kindPointer:
		genUnmarshalPtr(buf, f, ind2, ind3, valExpr)

	case kindArray:
		fmt.Fprintf(buf, "%s{\n", ind2)
		fmt.Fprintf(buf, "%sif %s.Type == 0x0A {\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sz.%s = nil\n", ind3+"\t", f.Name)
		fmt.Fprintf(buf, "%sbreak\n", ind3+"\t")
		fmt.Fprintf(buf, "%s}\n", ind3)
		fmt.Fprintf(buf, "%sarrBytes, ok := %s.ArrayOK()\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind3, "字段 %s 不是数组类型", f.BsonKey)
		fmt.Fprintf(buf, "%sarrElems, err := bsoncore.Document(arrBytes).Elements()\n", ind3)
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind3)
		fmt.Fprintf(buf, "%sz.%s = make(%s, 0, len(arrElems))\n", ind3, f.Name, f.GoType)
		fmt.Fprintf(buf, "%sfor _, ae := range arrElems {\n", ind3)
		genUnmarshalArrayElem(buf, f.ElemCat, f.Name, ind3+"\t")
		fmt.Fprintf(buf, "%s}\n", ind3)
		fmt.Fprintf(buf, "%s}\n", ind2)

	case kindMap:
		fmt.Fprintf(buf, "%s{\n", ind2)
		fmt.Fprintf(buf, "%sif %s.Type == 0x0A {\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sz.%s = nil\n", ind3+"\t", f.Name)
		fmt.Fprintf(buf, "%sbreak\n", ind3+"\t")
		fmt.Fprintf(buf, "%s}\n", ind3)
		fmt.Fprintf(buf, "%smapBytes, ok := %s.DocumentOK()\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind3, "字段 %s 不是文档类型", f.BsonKey)
		fmt.Fprintf(buf, "%smapElems, err := bsoncore.Document(mapBytes).Elements()\n", ind3)
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind3)
		fmt.Fprintf(buf, "%sz.%s = make(%s, len(mapElems))\n", ind3, f.Name, f.GoType)
		fmt.Fprintf(buf, "%sfor _, me := range mapElems {\n", ind3)
		genUnmarshalMapElem(buf, f.ElemCat, f.Name, ind3+"\t")
		fmt.Fprintf(buf, "%s}\n", ind3)
		fmt.Fprintf(buf, "%s}\n", ind2)

	case kindPrimitiveD, kindPrimitiveM:
		fmt.Fprintf(buf, "%s{\n", ind2)
		fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind3, "字段 %s 不是文档类型", f.BsonKey)
		fmt.Fprintf(buf, "%serr = bson.Unmarshal(subBytes, &z.%s)\n", ind3, f.Name)
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind3)
		fmt.Fprintf(buf, "%s}\n", ind2)
	case kindPrimitiveA:
		fmt.Fprintf(buf, "%s{\n", ind2)
		fmt.Fprintf(buf, "%sarrBytes, ok := %s.ArrayOK()\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind3, "字段 %s 不是数组类型", f.BsonKey)
		fmt.Fprintf(buf, "%serr = bson.Unmarshal(arrBytes, &z.%s)\n", ind3, f.Name)
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind3)
		fmt.Fprintf(buf, "%s}\n", ind2)

	case kindStructRef:
		fmt.Fprintf(buf, "%s{\n", ind2)
		if unmarshalerTypes[f.StructName] {
			fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind3, valExpr)

			fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind3, "字段 %s 不是文档类型", f.BsonKey)
			fmt.Fprintf(buf, "%sif err := z.%s.UnmarshalBSON(subBytes); err != nil { return err }\n", ind3, f.Name)
		} else {
			genUnmarshalStructValue(buf, f.Fields, valExpr, "z."+f.Name, ind3, f.BsonKey)
		}
		fmt.Fprintf(buf, "%s}\n", ind2)
	case kindAnonStruct:
		fmt.Fprintf(buf, "%s{\n", ind2)
		fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q) }\n", ind3, "field is not document")
		fmt.Fprintf(buf, "%ssubElems, err := bsoncore.Document(subBytes).Elements()\n", ind3)
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind3)
		fmt.Fprintf(buf, "%sfor _, se := range subElems {\n", ind3)
		fmt.Fprintf(buf, "%sswitch se.Key() {\n", ind3+"	")
		// For each sub-field, generate the appropriate case
		for _, sf := range f.Fields {
			genAnonFieldRead(buf, &sf, ind3+"		", f.Name)
		}
		fmt.Fprintf(buf, "%s}\n", ind3+"	") // switch se.Key()
		fmt.Fprintf(buf, "%s}\n", ind3)     // for _, se := range
		fmt.Fprintf(buf, "%s}\n", ind2)     // outer {}
	default:
		fmt.Fprintf(buf, "%s{\n", ind2)
		fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind3, valExpr)

		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q) }\n", ind3, "field is not document")
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &z.%s); err != nil { return err }\n", ind3, f.Name)
		fmt.Fprintf(buf, "%s}\n", ind2)
	}
}

func genUnmarshalStructValue(buf *bytes.Buffer, fields []fieldInfo, valueExpr, targetExpr, ind, key string) {
	subBytes := nextTmp("subBytes")
	subOK := nextTmp("subOK")
	subElems := nextTmp("subElems")
	subErr := nextTmp("subErr")
	subElem := nextTmp("subElem")
	fmt.Fprintf(buf, "%s%s, %s := %s.DocumentOK()\n", ind, subBytes, subOK, valueExpr)
	fmt.Fprintf(buf, "%sif !%s { return fmt.Errorf(%q, %q) }\n", ind, subOK, "字段 %s 不是文档类型", key)
	fmt.Fprintf(buf, "%s%s, %s := bsoncore.Document(%s).Elements()\n", ind, subElems, subErr, subBytes)
	fmt.Fprintf(buf, "%sif %s != nil { return %s }\n", ind, subErr, subErr)
	fmt.Fprintf(buf, "%sfor _, %s := range %s {\n", ind, subElem, subElems)
	fmt.Fprintf(buf, "%sswitch %s.Key() {\n", ind+"\t", subElem)
	for _, sf := range fields {
		if sf.BsonKey == "" {
			continue
		}
		fmt.Fprintf(buf, "%scase %q:\n", ind+"\t\t", sf.BsonKey)
		genUnmarshalAssign(buf, &sf, subElem+".Value()", targetExpr+"."+sf.Name, ind+"\t\t\t", sf.BsonKey)
	}
	fmt.Fprintf(buf, "%s}\n", ind+"\t")
	fmt.Fprintf(buf, "%s}\n", ind)
}

func genUnmarshalAssign(buf *bytes.Buffer, f *fieldInfo, valueExpr, targetExpr, ind, key string) {
	switch f.Category {
	case kindDouble:
		fmt.Fprintf(buf, "%s%s = %s.Double()\n", ind, targetExpr, valueExpr)
	case kindString:
		fmt.Fprintf(buf, "%s%s = %s.StringValue()\n", ind, targetExpr, valueExpr)
	case kindBoolean:
		fmt.Fprintf(buf, "%s%s = %s.Boolean()\n", ind, targetExpr, valueExpr)
	case kindInt32:
		fmt.Fprintf(buf, "%s%s = %s.Int32()\n", ind, targetExpr, valueExpr)
	case kindInt64:
		fmt.Fprintf(buf, "%s%s = %s.AsInt64()\n", ind, targetExpr, valueExpr)
	case kindInt:
		fmt.Fprintf(buf, "%s%s = int(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindInt8:
		fmt.Fprintf(buf, "%s%s = int8(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindInt16:
		fmt.Fprintf(buf, "%s%s = int16(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindUint:
		fmt.Fprintf(buf, "%s%s = uint(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindUint16:
		fmt.Fprintf(buf, "%s%s = uint16(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindUint32:
		fmt.Fprintf(buf, "%s%s = uint32(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindUint64:
		fmt.Fprintf(buf, "%s%s = uint64(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindByte:
		fmt.Fprintf(buf, "%s%s = byte(%s.AsInt64())\n", ind, targetExpr, valueExpr)
	case kindFloat32:
		fmt.Fprintf(buf, "%s%s = float32(%s.Double())\n", ind, targetExpr, valueExpr)
	case kindDateTime:
		fmt.Fprintf(buf, "%s%s = primitive.DateTime(%s.DateTime()).Time()\n", ind, targetExpr, valueExpr)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%s%s = primitive.DateTime(%s.DateTime())\n", ind, targetExpr, valueExpr)
	case kindObjectID:
		fmt.Fprintf(buf, "%s%s = %s.ObjectID()\n", ind, targetExpr, valueExpr)
	case kindBinary:
		if f.BinaryIsNative {
			fmt.Fprintf(buf, "%sif %s.Type == 0x0A {\n", ind, valueExpr)
			fmt.Fprintf(buf, "%s%s = nil\n", ind+"\t", targetExpr)
			fmt.Fprintf(buf, "%s} else {\n", ind)
			fmt.Fprintf(buf, "%s_, %s, _ = %s.BinaryOK()\n", ind+"\t", targetExpr, valueExpr)
			fmt.Fprintf(buf, "%s}\n", ind)
		} else {
			fmt.Fprintf(buf, "%ssubtype, data, _ := %s.BinaryOK(); %s = primitive.Binary{Subtype: subtype, Data: data}\n", ind, valueExpr, targetExpr)
		}
	case kindStructRef:
		if unmarshalerTypes[f.StructName] {
			fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind, valueExpr)
			fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind, "字段 %s 不是文档类型", key)
			fmt.Fprintf(buf, "%sif err := %s.UnmarshalBSON(subBytes); err != nil { return err }\n", ind, targetExpr)
		} else {
			genUnmarshalStructValue(buf, f.Fields, valueExpr, targetExpr, ind, key)
		}
	default:
		fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind, valueExpr)
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q, %q) }\n", ind, "字段 %s 不是文档类型", key)
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &%s); err != nil { return err }\n", ind, targetExpr)
	}
}

func genAnonFieldRead(buf *bytes.Buffer, sf *fieldInfo, ind, parentName string) {
	fmt.Fprintf(buf, "%scase %q:\n", ind, sf.BsonKey)
	prefix := parentName + "." + sf.Name
	switch sf.Category {
	case kindDouble:
		fmt.Fprintf(buf, "%sz.%s = se.Value().Double()\n", ind+"\t", prefix)
	case kindString:
		fmt.Fprintf(buf, "%sz.%s = se.Value().StringValue()\n", ind+"\t", prefix)
	case kindBoolean:
		fmt.Fprintf(buf, "%sz.%s = se.Value().Boolean()\n", ind+"\t", prefix)
	case kindInt32:
		fmt.Fprintf(buf, "%sz.%s = se.Value().Int32()\n", ind+"\t", prefix)
	case kindInt64:
		fmt.Fprintf(buf, "%sz.%s = se.Value().AsInt64()\n", ind+"\t", prefix)
	case kindInt:
		fmt.Fprintf(buf, "%sz.%s = int(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindInt8:
		fmt.Fprintf(buf, "%sz.%s = int8(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindInt16:
		fmt.Fprintf(buf, "%sz.%s = int16(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindUint:
		fmt.Fprintf(buf, "%sz.%s = uint(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindUint16:
		fmt.Fprintf(buf, "%sz.%s = uint16(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindUint32:
		fmt.Fprintf(buf, "%sz.%s = uint32(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindUint64:
		fmt.Fprintf(buf, "%sz.%s = uint64(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindByte:
		fmt.Fprintf(buf, "%sz.%s = byte(se.Value().AsInt64())\n", ind+"\t", prefix)
	case kindFloat32:
		fmt.Fprintf(buf, "%sz.%s = float32(se.Value().Double())\n", ind+"\t", prefix)
	case kindDateTime:
		fmt.Fprintf(buf, "%sz.%s = primitive.DateTime(se.Value().DateTime()).Time()\n", ind+"\t", prefix)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%sz.%s = primitive.DateTime(se.Value().DateTime())\n", ind+"\t", prefix)
	case kindObjectID:
		fmt.Fprintf(buf, "%sz.%s = se.Value().ObjectID()\n", ind+"\t", prefix)
	case kindBinary:
		if sf.BinaryIsNative {
			fmt.Fprintf(buf, "%s_, z.%s, _ = se.Value().BinaryOK()\n", ind+"\t", prefix)
		} else {
			fmt.Fprintf(buf, "%ssubtype, data, _ := se.Value().BinaryOK(); z.%s = primitive.Binary{Subtype: subtype, Data: data}\n", ind+"\t", prefix)
		}
	case kindRegex:
		fmt.Fprintf(buf, "%spattern, options := se.Value().Regex(); z.%s = primitive.Regex{Pattern: pattern, Options: options}\n", ind+"\t", prefix)
	case kindTimestamp:
		fmt.Fprintf(buf, "%st, i := se.Value().Timestamp(); z.%s = primitive.Timestamp{T: t, I: i}\n", ind+"\t", prefix)
	case kindDecimal128:
		fmt.Fprintf(buf, "%sz.%s = se.Value().Decimal128()\n", ind+"\t", prefix)
	case kindJavaScript:
		fmt.Fprintf(buf, "%sz.%s = primitive.JavaScript(se.Value().JavaScript())\n", ind+"\t", prefix)
	case kindSymbol:
		fmt.Fprintf(buf, "%sz.%s = primitive.Symbol(se.Value().Symbol())\n", ind+"\t", prefix)
	case kindArray:
		fmt.Fprintf(buf, "%s{\n", ind+"\t")
		fmt.Fprintf(buf, "%sif se.Value().Type == 0x0A { z.%s = nil; break }\n", ind+"\t\t", prefix)
		fmt.Fprintf(buf, "%sarrBytes, ok := se.Value().ArrayOK()\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q) }\n", ind+"\t\t", "field is not array")
		fmt.Fprintf(buf, "%sarrElems, err := bsoncore.Document(arrBytes).Elements()\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sz.%s = make(%s, 0, len(arrElems))\n", ind+"\t\t", prefix, sf.GoType)
		fmt.Fprintf(buf, "%sfor _, ae := range arrElems {\n", ind+"\t\t")
		genUnmarshalArrayElem(buf, sf.ElemCat, prefix, ind+"\t\t\t")
		fmt.Fprintf(buf, "%s}\n", ind+"\t\t")
		fmt.Fprintf(buf, "%s}\n", ind+"\t")
	case kindMap:
		fmt.Fprintf(buf, "%s{\n", ind+"\t")
		fmt.Fprintf(buf, "%sif se.Value().Type == 0x0A { z.%s = nil; break }\n", ind+"\t\t", prefix)
		fmt.Fprintf(buf, "%smapBytes, ok := se.Value().DocumentOK()\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q) }\n", ind+"\t\t", "field is not document")
		fmt.Fprintf(buf, "%smapElems, err := bsoncore.Document(mapBytes).Elements()\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sif err != nil { return err }\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sz.%s = make(%s, len(mapElems))\n", ind+"\t\t", prefix, sf.GoType)
		fmt.Fprintf(buf, "%sfor _, me := range mapElems {\n", ind+"\t\t")
		genUnmarshalMapElem(buf, sf.ElemCat, prefix, ind+"\t\t\t")
		fmt.Fprintf(buf, "%s}\n", ind+"\t\t")
		fmt.Fprintf(buf, "%s}\n", ind+"\t")
	case kindPointer:
		// For pointers in anonymous structs, we need special handling
		// For now, skip with a comment
		fmt.Fprintf(buf, "%s// pointer fields in anonymous structs not yet supported\n", ind+"\t")
	default:
		fmt.Fprintf(buf, "%s{\n", ind+"\t")
		fmt.Fprintf(buf, "%ssubSub, ok := se.Value().DocumentOK()\n", ind+"\t\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(%q) }\n", ind+"\t\t", "field is not document")
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subSub, &z.%s); err != nil { return err }\n", ind+"\t\t", prefix)
		fmt.Fprintf(buf, "%s}\n", ind+"\t")
	}
}
func genUnmarshalPtr(buf *bytes.Buffer, f *fieldInfo, ind2, ind3, valExpr string) {
	elem := f.ElemCat
	fmt.Fprintf(buf, "%s{\n", ind2)
	fmt.Fprintf(buf, "%sif %s.Type == 0x0A {\n", ind3, valExpr)
	fmt.Fprintf(buf, "%sz.%s = nil\n", ind3+"\t", f.Name)
	fmt.Fprintf(buf, "%sbreak\n", ind3+"\t")
	fmt.Fprintf(buf, "%s}\n", ind3)
	if elem.Category == kindStructRef {
		fmt.Fprintf(buf, "%sdocVal, ok := %s.DocumentOK()\n", ind3, valExpr)
		fmt.Fprintf(buf, "%sif ok {\n", ind3)
		fmt.Fprintf(buf, "%sz.%s = new(%s)\n", ind3, f.Name, elem.GoType)
		if unmarshalerTypes[elem.StructName] {
			fmt.Fprintf(buf, "%sif err := z.%s.UnmarshalBSON(docVal); err != nil { return err }\n", ind3, f.Name)
		} else {
			genUnmarshalStructValue(buf, elem.Fields, valExpr, "z."+f.Name, ind3, f.BsonKey)
		}
		fmt.Fprintf(buf, "%s} else {\n", ind3)
		fmt.Fprintf(buf, "%sz.%s = nil\n", ind3, f.Name)
		fmt.Fprintf(buf, "%s}\n", ind3)
		fmt.Fprintf(buf, "%s}\n", ind2)
		return
	}
	switch elem.Category {
	case kindString:
		fmt.Fprintf(buf, "%stmp := %s.StringValue(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindInt32:
		fmt.Fprintf(buf, "%stmp := %s.Int32(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindInt64:
		fmt.Fprintf(buf, "%stmp := %s.AsInt64(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindInt:
		fmt.Fprintf(buf, "%stmp := int(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindInt8:
		fmt.Fprintf(buf, "%stmp := int8(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindInt16:
		fmt.Fprintf(buf, "%stmp := int16(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindUint:
		fmt.Fprintf(buf, "%stmp := uint(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindUint16:
		fmt.Fprintf(buf, "%stmp := uint16(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindUint32:
		fmt.Fprintf(buf, "%stmp := uint32(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindUint64:
		fmt.Fprintf(buf, "%stmp := uint64(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindByte:
		fmt.Fprintf(buf, "%stmp := byte(%s.AsInt64()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindBoolean:
		fmt.Fprintf(buf, "%stmp := %s.Boolean(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindDouble:
		fmt.Fprintf(buf, "%stmp := %s.Double(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindFloat32:
		fmt.Fprintf(buf, "%stmp := float32(%s.Double()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindDateTime:
		fmt.Fprintf(buf, "%stmp := primitive.DateTime(%s.DateTime()).Time(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%stmp := primitive.DateTime(%s.DateTime()); z.%s = &tmp\n", ind3, valExpr, f.Name)
	case kindObjectID:
		fmt.Fprintf(buf, "%stmp := %s.ObjectID(); z.%s = &tmp\n", ind3, valExpr, f.Name)
	default:
		fmt.Fprintf(buf, "%sz.%s = nil\n", ind3, f.Name)
	}
	fmt.Fprintf(buf, "%s}\n", ind2)
}

func genUnmarshalArrayElem(buf *bytes.Buffer, elem *fieldInfo, fieldName, ind string) {
	switch elem.Category {
	case kindInt32:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, ae.Value().Int32())\n", ind, fieldName, fieldName)
	case kindInt64:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, ae.Value().Int64())\n", ind, fieldName, fieldName)
	case kindString:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, ae.Value().StringValue())\n", ind, fieldName, fieldName)
	case kindDouble:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, ae.Value().Double())\n", ind, fieldName, fieldName)
	case kindBoolean:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, ae.Value().Boolean())\n", ind, fieldName, fieldName)
	case kindInt:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, int(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindInt8:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, int8(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindInt16:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, int16(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindUint:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, uint(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindUint16:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, uint16(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindUint32:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, uint32(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindUint64:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, uint64(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindByte:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, byte(ae.Value().AsInt64()))\n", ind, fieldName, fieldName)
	case kindFloat32:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, float32(ae.Value().Double()))\n", ind, fieldName, fieldName)
	case kindDateTime:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, primitive.DateTime(ae.Value().DateTime()).Time())\n", ind, fieldName, fieldName)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, primitive.DateTime(ae.Value().DateTime()))\n", ind, fieldName, fieldName)
	case kindObjectID:
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, ae.Value().ObjectID())\n", ind, fieldName, fieldName)
	case kindStructRef:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, ok := ae.Value().DocumentOK()\n", ind+"\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t")
		fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
		if unmarshalerTypes[elem.StructName] {
			fmt.Fprintf(buf, "%sif err := subItem.UnmarshalBSON(subBytes); err != nil { return err }\n", ind+"\t")
		} else {
			genUnmarshalStructValue(buf, elem.Fields, "ae.Value()", "subItem", ind+"\t", "数组元素")
		}
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, subItem)\n", ind, fieldName, fieldName)
		fmt.Fprintf(buf, "%s}\n", ind)
	case kindPointer:
		// Bug fix: []*KnownStruct — pointer to a structRef inside a slice.
		if elem.ElemCat != nil && elem.ElemCat.Category == kindStructRef {
			fmt.Fprintf(buf, "%s{\n", ind)
			fmt.Fprintf(buf, "%sif ae.Value().Type == 0x0A {\n", ind+"\t")
			fmt.Fprintf(buf, "%sz.%s = append(z.%s, nil)\n", ind+"\t\t", fieldName, fieldName)
			fmt.Fprintf(buf, "%s} else {\n", ind+"\t")
			fmt.Fprintf(buf, "%ssubBytes, ok := ae.Value().DocumentOK()\n", ind+"\t\t")
			fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t\t")
			fmt.Fprintf(buf, "%ssubItem := new(%s)\n", ind+"\t\t", elem.ElemCat.GoType)
			if unmarshalerTypes[elem.ElemCat.StructName] {
				fmt.Fprintf(buf, "%sif err := subItem.UnmarshalBSON(subBytes); err != nil { return err }\n", ind+"\t\t")
			} else {
				genUnmarshalStructValue(buf, elem.ElemCat.Fields, "ae.Value()", "subItem", ind+"\t\t", "数组元素")
			}
			fmt.Fprintf(buf, "%sz.%s = append(z.%s, subItem)\n", ind+"\t\t", fieldName, fieldName)
			fmt.Fprintf(buf, "%s}\n", ind+"\t")
			fmt.Fprintf(buf, "%s}\n", ind)
		} else {
			// Generic pointer element — fall back.
			fmt.Fprintf(buf, "%s{\n", ind)
			fmt.Fprintf(buf, "%ssubBytes, ok := ae.Value().DocumentOK()\n", ind+"\t")
			fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t")
			fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
			fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &subItem); err != nil { return err }\n", ind+"\t")
			fmt.Fprintf(buf, "%sz.%s = append(z.%s, subItem)\n", ind, fieldName, fieldName)
			fmt.Fprintf(buf, "%s}\n", ind)
		}
		case kindArray:
		// Inner slice element — pre-allocate and inline parse
		subArrBytes := nextTmp("subArrBytes")
		subArrOK := nextTmp("subArrOK")
		subArrElems := nextTmp("subArrElems")
		subArrErr := nextTmp("subArrErr")
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%s%s, %s := ae.Value().ArrayOK()\n", ind+"\t", subArrBytes, subArrOK)
		fmt.Fprintf(buf, "%sif !%s { return fmt.Errorf(\"数组元素不是数组类型\") }\n", ind+"\t", subArrOK)
		fmt.Fprintf(buf, "%s%s, %s := bsoncore.Document(%s).Elements()\n", ind+"\t", subArrElems, subArrErr, subArrBytes)
		fmt.Fprintf(buf, "%sif %s != nil { return %s }\n", ind+"\t", subArrErr, subArrErr)
		subItem := nextTmp("subItem")
		fmt.Fprintf(buf, "%s%s := make(%s, 0, len(%s))\n", ind+"\t", subItem, elem.GoType, subArrElems)
		fmt.Fprintf(buf, "%sfor _, sae := range %s {\n", ind+"\t", subArrElems)
		genAppendValue(buf, elem.ElemCat, subItem, "sae.Value()", ind+"\t\t")
		fmt.Fprintf(buf, "%s}\n", ind+"\t")
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, %s)\n", ind, fieldName, fieldName, subItem)
		fmt.Fprintf(buf, "%s}\n", ind)

	case kindMap:
		// Inner map element — pre-allocate and inline parse
		subMapBytes := nextTmp("subMapBytes")
		subMapOK := nextTmp("subMapOK")
		subMapElems := nextTmp("subMapElems")
		subMapErr := nextTmp("subMapErr")
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%s%s, %s := ae.Value().DocumentOK()\n", ind+"\t", subMapBytes, subMapOK)
		fmt.Fprintf(buf, "%sif !%s { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t", subMapOK)
		fmt.Fprintf(buf, "%s%s, %s := bsoncore.Document(%s).Elements()\n", ind+"\t", subMapElems, subMapErr, subMapBytes)
		fmt.Fprintf(buf, "%sif %s != nil { return %s }\n", ind+"\t", subMapErr, subMapErr)
		subItem := nextTmp("subItem")
		fmt.Fprintf(buf, "%s%s := make(%s, len(%s))\n", ind+"\t", subItem, elem.GoType, subMapElems)
		fmt.Fprintf(buf, "%sfor _, sme := range %s {\n", ind+"\t", subMapElems)
		genMapAssignValue(buf, elem.ElemCat, subItem, "sme.Value()", "sme.Key()", ind+"\t\t")
		fmt.Fprintf(buf, "%s}\n", ind+"\t")
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, %s)\n", ind, fieldName, fieldName, subItem)
		fmt.Fprintf(buf, "%s}\n", ind)

	default:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, ok := ae.Value().DocumentOK()\n", ind+"\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t")
		fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &subItem); err != nil { return err }\n", ind+"\t")
		fmt.Fprintf(buf, "%sz.%s = append(z.%s, subItem)\n", ind, fieldName, fieldName)
		fmt.Fprintf(buf, "%s}\n", ind)
	}
}


// genAppendValue generates "target = append(target, value)" for a scalar value expression.
// Used for inlining inner array/map element parsing with pre-allocation.
func genAppendValue(buf *bytes.Buffer, elem *fieldInfo, target, valExpr, ind string) {
	switch elem.Category {
	case kindInt32:
		fmt.Fprintf(buf, "%s%s = append(%s, %s.Int32())\n", ind, target, target, valExpr)
	case kindInt64:
		fmt.Fprintf(buf, "%s%s = append(%s, %s.AsInt64())\n", ind, target, target, valExpr)
	case kindString:
		fmt.Fprintf(buf, "%s%s = append(%s, %s.StringValue())\n", ind, target, target, valExpr)
	case kindDouble:
		fmt.Fprintf(buf, "%s%s = append(%s, %s.Double())\n", ind, target, target, valExpr)
	case kindBoolean:
		fmt.Fprintf(buf, "%s%s = append(%s, %s.Boolean())\n", ind, target, target, valExpr)
	case kindInt:
		fmt.Fprintf(buf, "%s%s = append(%s, int(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindInt8:
		fmt.Fprintf(buf, "%s%s = append(%s, int8(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindInt16:
		fmt.Fprintf(buf, "%s%s = append(%s, int16(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindUint:
		fmt.Fprintf(buf, "%s%s = append(%s, uint(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindUint16:
		fmt.Fprintf(buf, "%s%s = append(%s, uint16(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindUint32:
		fmt.Fprintf(buf, "%s%s = append(%s, uint32(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindUint64:
		fmt.Fprintf(buf, "%s%s = append(%s, uint64(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindByte:
		fmt.Fprintf(buf, "%s%s = append(%s, byte(%s.AsInt64()))\n", ind, target, target, valExpr)
	case kindFloat32:
		fmt.Fprintf(buf, "%s%s = append(%s, float32(%s.Double()))\n", ind, target, target, valExpr)
	case kindDateTime:
		fmt.Fprintf(buf, "%s%s = append(%s, primitive.DateTime(%s.DateTime()).Time())\n", ind, target, target, valExpr)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%s%s = append(%s, primitive.DateTime(%s.DateTime()))\n", ind, target, target, valExpr)
	case kindObjectID:
		fmt.Fprintf(buf, "%s%s = append(%s, %s.ObjectID())\n", ind, target, target, valExpr)
	default:
		// For complex types (arrays inside arrays etc.), fall back to bson.Unmarshal
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind+"\t", valExpr)
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t")
		fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &subItem); err != nil { return err }\n", ind+"\t")
		fmt.Fprintf(buf, "%s%s = append(%s, subItem)\n", ind, target, target)
		fmt.Fprintf(buf, "%s}\n", ind)
	}
}

// genMapAssignValue generates "target[key] = value" for a map value expression.
// Used for inlining inner map element parsing with pre-allocation.
func genMapAssignValue(buf *bytes.Buffer, elem *fieldInfo, target, valExpr, keyExpr, ind string) {
	switch elem.Category {
	case kindInt32:
		fmt.Fprintf(buf, "%s%s[%s] = %s.Int32()\n", ind, target, keyExpr, valExpr)
	case kindInt64:
		fmt.Fprintf(buf, "%s%s[%s] = %s.AsInt64()\n", ind, target, keyExpr, valExpr)
	case kindString:
		fmt.Fprintf(buf, "%s%s[%s] = %s.StringValue()\n", ind, target, keyExpr, valExpr)
	case kindDouble:
		fmt.Fprintf(buf, "%s%s[%s] = %s.Double()\n", ind, target, keyExpr, valExpr)
	case kindBoolean:
		fmt.Fprintf(buf, "%s%s[%s] = %s.Boolean()\n", ind, target, keyExpr, valExpr)
	case kindInt:
		fmt.Fprintf(buf, "%s%s[%s] = int(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindInt8:
		fmt.Fprintf(buf, "%s%s[%s] = int8(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindInt16:
		fmt.Fprintf(buf, "%s%s[%s] = int16(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindUint:
		fmt.Fprintf(buf, "%s%s[%s] = uint(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindUint16:
		fmt.Fprintf(buf, "%s%s[%s] = uint16(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindUint32:
		fmt.Fprintf(buf, "%s%s[%s] = uint32(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindUint64:
		fmt.Fprintf(buf, "%s%s[%s] = uint64(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindByte:
		fmt.Fprintf(buf, "%s%s[%s] = byte(%s.AsInt64())\n", ind, target, keyExpr, valExpr)
	case kindFloat32:
		fmt.Fprintf(buf, "%s%s[%s] = float32(%s.Double())\n", ind, target, keyExpr, valExpr)
	case kindDateTime:
		fmt.Fprintf(buf, "%s%s[%s] = primitive.DateTime(%s.DateTime()).Time()\n", ind, target, keyExpr, valExpr)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%s%s[%s] = primitive.DateTime(%s.DateTime())\n", ind, target, keyExpr, valExpr)
	case kindObjectID:
		fmt.Fprintf(buf, "%s%s[%s] = %s.ObjectID()\n", ind, target, keyExpr, valExpr)
	default:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, ok := %s.DocumentOK()\n", ind+"\t", valExpr)
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"数组元素不是文档类型\") }\n", ind+"\t")
		fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &subItem); err != nil { return err }\n", ind+"\t")
		fmt.Fprintf(buf, "%s%s[%s] = subItem\n", ind, target, keyExpr)
		fmt.Fprintf(buf, "%s}\n", ind)
	}
}

func genUnmarshalMapElem(buf *bytes.Buffer, elem *fieldInfo, fieldName, ind string) {
	switch elem.Category {
	case kindInt32:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = me.Value().Int32()\n", ind, fieldName)
	case kindInt64:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = me.Value().Int64()\n", ind, fieldName)
	case kindString:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = me.Value().StringValue()\n", ind, fieldName)
	case kindDouble:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = me.Value().Double()\n", ind, fieldName)
	case kindBoolean:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = me.Value().Boolean()\n", ind, fieldName)
	case kindInt:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = int(me.Value().AsInt64())\n", ind, fieldName)
	case kindInt8:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = int8(me.Value().AsInt64())\n", ind, fieldName)
	case kindInt16:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = int16(me.Value().AsInt64())\n", ind, fieldName)
	case kindUint:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = uint(me.Value().AsInt64())\n", ind, fieldName)
	case kindUint16:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = uint16(me.Value().AsInt64())\n", ind, fieldName)
	case kindUint32:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = uint32(me.Value().AsInt64())\n", ind, fieldName)
	case kindUint64:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = uint64(me.Value().AsInt64())\n", ind, fieldName)
	case kindByte:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = byte(me.Value().AsInt64())\n", ind, fieldName)
	case kindFloat32:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = float32(me.Value().Double())\n", ind, fieldName)
	case kindDateTime:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = primitive.DateTime(me.Value().DateTime()).Time()\n", ind, fieldName)
	case kindPrimitiveDateTime:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = primitive.DateTime(me.Value().DateTime())\n", ind, fieldName)
	case kindObjectID:
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = me.Value().ObjectID()\n", ind, fieldName)
	case kindStructRef:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, ok := me.Value().DocumentOK()\n", ind+"\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"map 元素不是文档类型\") }\n", ind+"\t")
		fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
		fmt.Fprintf(buf, "%sif err := subItem.UnmarshalBSON(subBytes); err != nil { return err }\n", ind+"\t")
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = subItem\n", ind, fieldName)
		fmt.Fprintf(buf, "%s}\n", ind)
	default:
		fmt.Fprintf(buf, "%s{\n", ind)
		fmt.Fprintf(buf, "%ssubBytes, ok := me.Value().DocumentOK()\n", ind+"\t")
		fmt.Fprintf(buf, "%sif !ok { return fmt.Errorf(\"map 元素不是文档类型\") }\n", ind+"\t")
		fmt.Fprintf(buf, "%svar subItem %s\n", ind+"\t", elem.GoType)
		fmt.Fprintf(buf, "%sif err := bson.Unmarshal(subBytes, &subItem); err != nil { return err }\n", ind+"\t")
		fmt.Fprintf(buf, "%sz.%s[me.Key()] = subItem\n", ind, fieldName)
		fmt.Fprintf(buf, "%s}\n", ind)
	}
}
