package jade

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"github.com/go-floki/jade/parser"
	"github.com/go-floki/jade/path"
	"go/ast"
	gp "go/parser"
	gt "go/token"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var builtinFunctions = [...]string{
	"len",
	"print",
	"printf",
	"println",
	"urlquery",
	"js",
	"json",
	"index",
	"html",
	"unescaped",
}

// Compiler is the main interface of Amber Template Engine.
// In order to use an Amber template, it is required to create a Compiler and
// compile an Amber source to native Go template.
//	compiler := jade.New()
// 	// Parse the input file
//	err := compiler.ParseFile("./input.jade")
//	if err == nil {
//		// Compile input file to Go template
//		tpl, err := compiler.Compile()
//		if err == nil {
//			// Check built in html/template documentation for further details
//			tpl.Execute(os.Stdout, somedata)
//		}
//	}
type Compiler struct {
	// Compiler options
	Options
	filename     string
	node         parser.Node
	indentLevel  int
	newline      bool
	buffer       *bytes.Buffer
	tempvarIndex int
	mixins       map[string]*parser.Mixin
}

// Create and initialize a new Compiler
func New() *Compiler {
	compiler := new(Compiler)
	compiler.filename = ""
	compiler.tempvarIndex = 0
	compiler.PrettyPrint = true
	compiler.Options = DefaultOptions
	compiler.mixins = make(map[string]*parser.Mixin)

	return compiler
}

type Options struct {
	// Setting if pretty printing is enabled.
	// Pretty printing ensures that the output html is properly indented and in human readable form.
	// If disabled, produced HTML is compact. This might be more suitable in production environments.
	// Default: true
	PrettyPrint bool
	// Setting if line number emitting is enabled
	// In this form, Amber emits line number comments in the output template. It is usable in debugging environments.
	// Default: false
	LineNumbers bool
	// Templates source file system
	// Default: http.Dir("")
	Fs http.FileSystem
	// The path separator to use for the file system.
	// Enables using with go:embed file system, which has strictly '/' as the path separator.
	// Default: os.PathSeparator
	PathSeparator rune

    // Custom functions
    Funcs template.FuncMap
}

// Used to provide options to directory compilation
type DirOptions struct {
	// File extension to match for compilation
	Ext string
	// Whether or not to walk subdirectories
	Recursive bool
}

var DefaultOptions = Options{true, false, http.Dir(""), os.PathSeparator, nil}
var DefaultDirOptions = DirOptions{".jade", true}

// Parses and compiles the supplied jade template string. Returns corresponding Go Template (html/templates) instance.
// Necessary runtime functions will be injected and the template will be ready to be executed.
func Compile(input string, options Options) (*template.Template, error) {
	comp := New()
	comp.Options = options

	err := comp.Parse(input)
	if err != nil {
		return nil, err
	}

	return comp.Compile()
}

// Parses and compiles the contents of supplied filename. Returns corresponding Go Template (html/templates) instance.
// Necessary runtime functions will be injected and the template will be ready to be executed.
func CompileFile(filename string, options Options) (*template.Template, error) {
	comp := New()
	comp.Options = options

	err := comp.ParseFile(filename)
	if err != nil {
		return nil, err
	}

	return comp.Compile()
}

// Parses and compiles the contents of a supplied directory path, with options.
// Returns a map of a template identifier (key) to a Go Template instance.
// Ex: if the dirname="templates/" had a file "index.jade" the key would be "index"
// If option for recursive is True, this parses every file of relevant extension
// in all subdirectories. The key then is the path e.g: "layouts/layout"
func CompileDir(dirname string, dopt DirOptions, opt Options) (map[string]*template.Template, error) {
	dir, err := opt.Fs.Open(dirname)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	files, err := dir.Readdir(0)
	if err != nil {
		return nil, err
	}

	compiled := make(map[string]*template.Template)
	for _, file := range files {
		// filename is for example "index.jade"
		filename := file.Name()
		fileext := filepath.Ext(filepath.Clean(filename))

		// If recursive is true and there's a subdirectory, recurse
		if dopt.Recursive && file.IsDir() {
			dirpath := path.Join(opt.PathSeparator, dirname, filename)
			subcompiled, err := CompileDir(dirpath, dopt, opt)
			if err != nil {
				return nil, err
			}
			// Copy templates from subdirectory into parent template mapping
			for k, v := range subcompiled {
				// Concat with parent directory name for unique paths
				key := path.Join(opt.PathSeparator, filename, k)
				compiled[key] = v
			}
		} else if fileext == dopt.Ext {
			// Otherwise compile the file and add to mapping
			fullpath := path.Join(opt.PathSeparator, dirname, filename)
			tmpl, err := CompileFile(fullpath, opt)
			if err != nil {
				return nil, err
			}
			// Strip extension
			key := filename[0 : len(filename)-len(fileext)]
			compiled[key] = tmpl
		}
	}

	return compiled, nil
}

// Parse given raw jade template string.
func (c *Compiler) Parse(input string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(r.(string))
		}
	}()

	parser, err := parser.StringParser(input)
    parser.FileName(c.filename)

	if err != nil {
		return
	}

	c.node = parser.Parse()
	return
}

// Parse the jade template file in given path
func (c *Compiler) ParseFile(filename string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(r.(string))
		}
	}()

	parser, err := parser.FileParserFs(c.Options.Fs, c.Options.PathSeparator, filename)

	if err != nil {
		return
	}

	c.node = parser.Parse()
	c.filename = filename
	return
}

// Compile jade and create a Go Template (html/templates) instance.
// Necessary runtime functions will be injected and the template will be ready to be executed.
func (c *Compiler) Compile() (*template.Template, error) {
	return c.CompileWithName(path.Convert(c.Options.PathSeparator, c.filename, filepath.Base))
}

// Same as Compile but allows to specify a name for the template
func (c *Compiler) CompileWithName(name string) (*template.Template, error) {
	return c.CompileWithTemplate(template.New(name))
}

// Same as Compile but allows to specify a template
func (c *Compiler) CompileWithTemplate(t *template.Template) (*template.Template, error) {
	data, err := c.CompileString()
	if err != nil {
		return nil, err
	}

	tpl, err := t.Funcs(FuncMap).Funcs(c.Options.Funcs).Parse(data)
	if err != nil {
		return nil, err
	}

	return tpl, nil
}

// Compile jade and write the Go Template source into given io.Writer instance
// You would not be using this unless debugging / checking the output. Please use Compile
// method to obtain a template instance directly.
func (c *Compiler) CompileWriter(out io.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(r.(string))
		}
	}()

	c.buffer = new(bytes.Buffer)
	c.visit(c.node)

	if c.buffer.Len() > 0 {
		c.write("\n")
	}

	_, err = c.buffer.WriteTo(out)
	return
}

// Compile template and return the Go Template source
// You would not be using this unless debugging / checking the output. Please use Compile
// method to obtain a template instance directly.
func (c *Compiler) CompileString() (string, error) {
	var buf bytes.Buffer

	if err := c.CompileWriter(&buf); err != nil {
		return "", err
	}

	result := buf.String()

	return result, nil
}

func (c *Compiler) visit(node parser.Node) {
	defer func() {
		if r := recover(); r != nil {
			if rs, ok := r.(string); ok && rs[:len("Jade Error")] == "Jade Error" {
				panic(r)
			}

			pos := node.Pos()

			if len(pos.Filename) > 0 {
				panic(fmt.Sprintf("Jade Error in <%s>: %v - Line: %d, Column: %d, Length: %d", pos.Filename, r, pos.LineNum, pos.ColNum, pos.TokenLength))
			} else {
				panic(fmt.Sprintf("Jade Error: %v - Line: %d, Column: %d, Length: %d", r, pos.LineNum, pos.ColNum, pos.TokenLength))
			}
		}
	}()

	switch node.(type) {
	case *parser.Block:
		c.visitBlock(node.(*parser.Block))
	case *parser.Doctype:
		c.visitDoctype(node.(*parser.Doctype))
	case *parser.Comment:
		c.visitComment(node.(*parser.Comment))
	case *parser.Tag:
		c.visitTag(node.(*parser.Tag))
	case *parser.Text:
		c.visitText(node.(*parser.Text))
	case *parser.Condition:
		c.visitCondition(node.(*parser.Condition))
	case *parser.Each:
		c.visitEach(node.(*parser.Each))
	case *parser.Buffered:
		c.visitBuffered(node.(*parser.Buffered))
	case *parser.Assignment:
		c.visitAssignment(node.(*parser.Assignment))
	case *parser.Mixin:
		c.visitMixin(node.(*parser.Mixin))
	case *parser.MixinCall:
		c.visitMixinCall(node.(*parser.MixinCall))
	}
}

func (c *Compiler) write(value string) {
	c.buffer.WriteString(value)
}

func (c *Compiler) indent(offset int, newline bool) {
	if !c.PrettyPrint {
		return
	}

	if newline && c.buffer.Len() > 0 {
		c.write("\n")
	}

	for i := 0; i < c.indentLevel+offset; i++ {
		c.write("\t")
	}
}

func (c *Compiler) tempvar() string {
	c.tempvarIndex++
	return "$__jade_" + strconv.Itoa(c.tempvarIndex)
}

func (c *Compiler) escape(input string) string {
	return strings.Replace(strings.Replace(input, `\`, `\\`, -1), `"`, `\"`, -1)
}

func (c *Compiler) visitBlock(block *parser.Block) {
	for _, node := range block.Children {
		if _, ok := node.(*parser.Text); !block.CanInline() && ok {
			c.indent(0, true)
		}

		c.visit(node)
	}
}

func (c *Compiler) visitDoctype(doctype *parser.Doctype) {
	c.write(doctype.String())
}

func (c *Compiler) visitComment(comment *parser.Comment) {
	if comment.Silent {
		return
	}

	c.indent(0, false)

	if comment.Block == nil {
		c.write(`{{unescaped "<!-- ` + c.escape(comment.Value) + ` -->"}}`)
	} else {
		c.write(`<!-- ` + comment.Value)
		c.visitBlock(comment.Block)
		c.write(` -->`)
	}
}

func (c *Compiler) visitCondition(condition *parser.Condition) {
	c.write(`{{if ` + c.visitRawInterpolation(condition.Expression) + `}}`)
	c.visitBlock(condition.Positive)
	if condition.Negative != nil {
		c.write(`{{else}}`)
		c.visitBlock(condition.Negative)
	}
	c.write(`{{end}}`)
}

func (c *Compiler) visitEach(each *parser.Each) {
	if each.Block == nil {
		return
	}

	if len(each.Y) == 0 {
		c.write(`{{range ` + each.X + ` := ` + c.visitRawInterpolation(each.Expression) + `}}`)
	} else {
		c.write(`{{range ` + each.X + `, ` + each.Y + ` := ` + c.visitRawInterpolation(each.Expression) + `}}`)
	}
	c.visitBlock(each.Block)
	c.write(`{{end}}`)
}

func (c *Compiler) visitBuffered(buff *parser.Buffered) {
	if buff.Escaped {
		c.write(`{{` + c.visitRawInterpolation(buff.Expression) + `}}`)
	} else {
		c.write(`{{unescaped ` + c.visitRawInterpolation(buff.Expression) + `}}`)
	}
}

func (c *Compiler) visitAssignment(assgn *parser.Assignment) {
	c.write(`{{` + assgn.X + ` := ` + c.visitRawInterpolation(assgn.Expression) + `}}`)
}

func (c *Compiler) visitTag(tag *parser.Tag) {
	type attrib struct {
		name      string
		value     string
		condition string
	}

	attribs := make(map[string]*attrib)

	for _, item := range tag.Attributes {
		attr := new(attrib)
		attr.name = item.Name

		if !item.IsRaw {
			if strings.Index(attr.name, "on") == 0 {
				attr.value = c.visitJSInterpolation(item.Value)
			} else {
				attr.value = c.visitInterpolation(item.Value)
			}

		} else if item.Value == "" {
			attr.value = ""
		} else {
			if item.IsRaw {
				attr.value = item.Value
			} else {
				attr.value = `{{"` + item.Value + `"}}`
			}
		}

		if len(item.Condition) != 0 {
			attr.condition = c.visitRawInterpolation(item.Condition)
		}

		if attr.name == "class" && attribs["class"] != nil {
			prevclass := attribs["class"]
			attr.value = ` ` + attr.value

			if len(attr.condition) > 0 {
				attr.value = `{{if ` + attr.condition + `}}` + attr.value + `{{end}}`
				attr.condition = ""
			}

			if len(prevclass.condition) > 0 {
				prevclass.value = `{{if ` + prevclass.condition + `}}` + prevclass.value + `{{end}}`
				prevclass.condition = ""
			}

			prevclass.value = prevclass.value + attr.value
		} else {
			attribs[item.Name] = attr
		}
	}

	c.indent(0, true)
	c.write("<" + tag.Name)

	var attrNames []string
	for k := range attribs {
		attrNames = append(attrNames, k)
	}

	sort.Strings(attrNames)

	for attrIdx := range attrNames {
		name := attrNames[attrIdx]
		value := attribs[name]
		if len(value.condition) > 0 {
			c.write(`{{if ` + value.condition + `}}`)
		}

		if value.value == "" {
			c.write(` ` + name)
		} else {
			c.write(` ` + name + `="` + value.value + `"`)
		}

		if len(value.condition) > 0 {
			c.write(`{{end}}`)
		}
	}

	if tag.IsSelfClosing() {
		c.write(` />`)
	} else {
		c.write(`>`)

		if tag.Block != nil {
			if !tag.Block.CanInline() {
				c.indentLevel++
			}

			c.visitBlock(tag.Block)

			if !tag.Block.CanInline() {
				c.indentLevel--
				c.indent(0, true)
			}
		}

		c.write(`</` + tag.Name + `>`)
	}
}

var textInterpolateRegexp = regexp.MustCompile(`#\{(.*?)\}`)
var textEscapeRegexp = regexp.MustCompile(`\{\{(.*?)\}\}`)

func (c *Compiler) visitText(txt *parser.Text) {
	value := textEscapeRegexp.ReplaceAllStringFunc(txt.Value, func(value string) string {
		return `{{"{{"}}` + value[2:len(value)-2] + `{{"}}"}}`
	})

	value = textInterpolateRegexp.ReplaceAllStringFunc(value, func(value string) string {
		return c.visitInterpolation(value[2 : len(value)-1])
	})

	lines := strings.Split(value, "\n")
	for i := 0; i < len(lines); i++ {
		c.write(lines[i])

		if i < len(lines)-1 {
			c.write("\n")
			c.indent(0, false)
		}
	}
}

func (c *Compiler) visitInterpolation(value string) string {
	return `{{` + c.visitRawInterpolation(value) + `}}`
}

func (c *Compiler) visitJSInterpolation(value string) string {
	return `{{ safeJS ` + c.visitRawInterpolation(value) + `}}`
}

func (c *Compiler) visitRawInterpolation(value string) string {
	value = strings.Replace(value, "$", "__DOLLAR__", -1)
	expr, err := gp.ParseExpr(value)

    if err != nil {
        // dumb hack to parse single-quoted strings
        valueLen := len(value)
        bValue := []byte(value)
        if bValue[0] == '\'' && bValue[valueLen - 1] == '\'' {
            bValue[0] = '"'
            bValue[valueLen - 1] = '"'
            expr, err = gp.ParseExpr(string(bValue))
        }
    }

	if err != nil {
        panic(fmt.Sprintf("Unable to parse expression: %s", value))
	}

	value = strings.Replace(c.visitExpression(expr), "__DOLLAR__", "$", -1)
	return value
}

func (c *Compiler) hasFunctionWithName(name string) bool {
    if _, inCustom := c.Options.Funcs[name]; inCustom {
        return true
    } else if _, inRuntime := FuncMap[name]; inRuntime {
        return true
    }

    return false
}

func (c *Compiler) visitExpression(outerexpr ast.Expr) string {
	stack := list.New()

	pop := func() string {
		if stack.Front() == nil {
			return ""
		}

		val := stack.Front().Value.(string)
		stack.Remove(stack.Front())
		return val
	}

	var exec func(ast.Expr)

	exec = func(expr ast.Expr) {
		switch expr.(type) {
		case *ast.BinaryExpr:
			{
				be := expr.(*ast.BinaryExpr)

				exec(be.Y)
				exec(be.X)

                name := c.tempvar()

                switch be.Op {
                    case gt.OR:
                    c.write(`{{` + name + ` := ` + pop() + ` | ` + pop() + `}}`)
                    stack.PushFront(name)
                    return
                }

                negate := false
				c.write(`{{` + name + ` := `)

				switch be.Op {
				case gt.ADD:
					c.write("__jade_add ")
				case gt.SUB:
					c.write("__jade_sub ")
				case gt.MUL:
					c.write("__jade_mul ")
				case gt.QUO:
					c.write("__jade_quo ")
				case gt.REM:
					c.write("__jade_rem ")
				case gt.LAND:
					c.write("and ")
				case gt.LOR:
					c.write("or ")
				case gt.EQL:
					c.write("__jade_eql ")
				case gt.NEQ:
					c.write("__jade_eql ")
					negate = true
				case gt.LSS:
					c.write("__jade_lss ")
				case gt.GTR:
					c.write("__jade_gtr ")
				case gt.LEQ:
					c.write("__jade_gtr ")
					negate = true
				case gt.GEQ:
					c.write("__jade_lss ")
					negate = true
				default:
					panic("Unexpected operator: '" + be.Op.String() + "'")
				}

				c.write(pop() + ` ` + pop() + `}}`)

				if !negate {
					stack.PushFront(name)
				} else {
					negname := c.tempvar()
					c.write(`{{` + negname + ` := not ` + name + `}}`)
					stack.PushFront(negname)
				}
			}
		case *ast.UnaryExpr:
			{
				ue := expr.(*ast.UnaryExpr)

				exec(ue.X)

				name := c.tempvar()
				c.write(`{{` + name + ` := `)

				switch ue.Op {
				case gt.SUB:
					c.write("__jade_minus ")
				case gt.ADD:
					c.write("__jade_plus ")
				case gt.NOT:
					c.write("not ")
				default:
					panic("Unexpected operator: '" + ue.Op.String() + "'")
				}

				c.write(pop() + `}}`)
				stack.PushFront(name)
			}
		case *ast.ParenExpr:
			exec(expr.(*ast.ParenExpr).X)
		case *ast.BasicLit:
			stack.PushFront(expr.(*ast.BasicLit).Value)
		case *ast.Ident:
			name := expr.(*ast.Ident).Name
			if len(name) >= len("__DOLLAR__") && name[:len("__DOLLAR__")] == "__DOLLAR__" {
				if name == "__DOLLAR__" {
					stack.PushFront(`.`)
				} else {
					stack.PushFront(`$` + expr.(*ast.Ident).Name[len("__DOLLAR__"):])
				}
			} else {
				rname := expr.(*ast.Ident).Name
				switch rname {
				case "nil":
					stack.PushFront(rname)
				default:
                    if c.hasFunctionWithName(rname) {
                        stack.PushFront(rname)
                    } else {
                        stack.PushFront(`.` + rname)
                    }
				}
			}
		case *ast.SelectorExpr:
			se := expr.(*ast.SelectorExpr)
			exec(se.X)
			x := pop()

			if x == "." {
				x = ""
			}

			name := c.tempvar()
			c.write(`{{` + name + ` := ` + x + `.` + se.Sel.Name + `}}`)
			stack.PushFront(name)

		case *ast.CallExpr:
			ce := expr.(*ast.CallExpr)

			for i := len(ce.Args) - 1; i >= 0; i-- {
				exec(ce.Args[i])
			}

			name := c.tempvar()
			builtin := false

			if ident, ok := ce.Fun.(*ast.Ident); ok {
                for _, fname := range builtinFunctions {
					if fname == ident.Name {
						builtin = true
						break
					}
				}
			}

			if builtin {
                // @todo check what's going on here..
				stack.PushFront(ce.Fun.(*ast.Ident).Name)
				c.write("{{" + name + " := " + pop())

			} else {
                if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
					exec(se.X)
					x := pop()

					if x == "." {
						x = ""
					}

                    c.write("{{" + name + " := " + x + "." + se.Sel.Name + " ")

				} else {
					exec(ce.Fun)
					c.write("{{" + name + " := " + pop())
				}

			}

			for i := 0; i < len(ce.Args); i++ {
				c.write(` `)
				c.write(pop())
			}

			c.write(`}}`)

			stack.PushFront(name)
		default:
			panic("Unable to parse expression. Unsupported: " + reflect.TypeOf(expr).String())
		}
	}

	exec(outerexpr)
	return pop()
}

func (c *Compiler) visitMixin(mixin *parser.Mixin) {
	c.mixins[mixin.Name] = mixin
}

func (c *Compiler) visitMixinCall(mixinCall *parser.MixinCall) {
	mixin := c.mixins[mixinCall.Name]
	for i, arg := range mixin.Args {
		c.write(fmt.Sprintf(`{{%s := %s}}`, arg, c.visitRawInterpolation(mixinCall.Args[i])))
	}
	c.visitBlock(mixin.Block)
}
