package jade

import (
	"bytes"
	"fmt"
	//"html/template"
	//"os"
	"strings"
	"testing"
    "io/ioutil"
    "flag"
    "html/template"
)

var (
    selCase = flag.String("case", "", "Specify test case name to run")

    customFuncs = template.FuncMap{
        "foo": func(x string) string {
            return x + "bar"
        },
        "bar": func() string {
            return "bar"
        },
    }
)

func init() {
    flag.Parse()
}


func Test_Cases(t *testing.T) {
    casesDir := "./test/cases"

    files, _ := ioutil.ReadDir(casesDir)
    for _, f := range files {
        name := f.Name()
        if strings.Index(name, ".jade") != -1 {
            if *selCase != "" && strings.Index(name, *selCase) == -1 {
                continue
            }

            fmt.Println("--- TEST:", name)

            jadeFile := casesDir + "/" + name

            contents, err := ioutil.ReadFile(jadeFile)
            if err != nil {
                t.Fatal(err)
            }

            htmlName := strings.Replace(name, ".jade", ".html", 1)
            expected, err := ioutil.ReadFile(casesDir + "/" + htmlName)
            if err != nil {
                t.Fatal(err)
            }

            res, compiledTpl, err := runPretty(jadeFile, string(contents), nil)
            if err != nil {
                t.Fatal(err)
            }

            expectedStr := strings.TrimSpace(string(expected))

            res = strings.TrimSpace(res)

            // replace TAB characters with 4 SPACE characters
            res = strings.Replace(res, "\t", "    ", -1)

            expectWithSource(res, expectedStr, compiledTpl, t)
        }
    }
}

func Test_Doctype(t *testing.T) {
	res, err := run(`!!! 5`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<!DOCTYPE html>`, t)
	}
}

func Test_Nesting(t *testing.T) {
	res, err := run(`html
						head
							title
						body
							div: p
							.anotherDiv: p`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<html><head><title></title></head><body><div><p></p></div><div class="anotherDiv"><p></p></div></body></html>`, t)
	}

	res, err = run(`body
					    span#popup
						.wrap.container
							#top.header`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<body><span id="popup"></span><div class="wrap container"><div class="header" id="top"></div></div></body>`, t)
	}
	_, err = run(`
	.cls
    `, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func Test_Mixin(t *testing.T) {
	res, err := run(`
		mixin a($a)
			p #{$a}

		+a(1)`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p>1</p>`, t)
	}
}

func Test_Mixin_NoArguments(t *testing.T) {
	res, err := run(`
		mixin a()
			p Testing

		+a()`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p>Testing</p>`, t)
	}
}

func Test_Mixin_MultiArguments(t *testing.T) {
	res, err := run(`
		mixin a($a, $b, $c, $d)
			p #{$a} #{$b} #{$c} #{$d}

		+a("a", "b", "c", A)`, map[string]int{"A": 2})

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p>a b c 2</p>`, t)
	}
}

func Test_ClassName(t *testing.T) {
	res, err := run(`.test
						p.test1.test-2
							[class=$]
							.test3`, "test4")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<div class="test"><p class="test1 test-2 test4"><div class="test3"></div></p></div>`, t)
	}
}

func Test_Id(t *testing.T) {
	res, err := run(`div#test
						p#test1#test2`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<div id="test"><p id="test2"></p></div>`, t)
	}
}

func Test_Attribute(t *testing.T) {
	res, err := run(`div[name="Test"]
						p
							[style="text-align: center"]`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<div name="Test"><p style="text-align: center"></p></div>`, t)
	}
}

func Test_Attribute2(t *testing.T) {
	res, err := run(`button[onclick="foobar()"] Label`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<button onclick="foobar()">Label</button>`, t)
	}
}

func Test_Attribute3(t *testing.T) {
	res, err := run(`button[onclick="foobar(" + $ + ")"] Label`, "1")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<button onclick="foobar(1)">Label</button>`, t)
	}
}

func Test_JadeAttribute(t *testing.T) {
	res, err := run(`button(title="test",style="text-align:center") Label`, "1")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect2(res, `<button style="text-align:center" title="test">Label</button>`,
			`<button title="test" style="text-align:center">Label</button>`, t)
	}
}

func Test_JadeAttribute2(t *testing.T) {
	res, err := run(`button(onclick="testfunc(" +$+ ")") Label`, "1")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<button onclick="testfunc(1)">Label</button>`, t)
	}
}

func Test_JadeAttributeEscaping(t *testing.T) {
	res, err := run(`p(style=$)`, "<div>")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p style="&lt;div&gt;"></p>`, t)
	}
}

func Test_BufferedCode(t *testing.T) {

	res, err := run(`p= "test"`, "")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p>test</p>`, t)
	}

	res, err = run(`= $`, "1")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `1`, t)
	}

	res, err = run(`= $`, "\"")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `&#34;`, t)
	}

	res, err = run(`p= $`, "\"")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p>&#34;</p>`, t)
	}

}

func Test_BufferedUnescapedCode(t *testing.T) {
	res, err := run(`!= $`, "%")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `%`, t)
	}

	res, err = run(`p!= $`, "%")

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p>%</p>`, t)
	}
}

func Test_Conditional(t *testing.T) {
	res, err := run(`if $
		p`, true)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<p></p>`, t)
	}

	res, err = run(`if $
		p
else
		i`, false)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<i></i>`, t)
	}

	res, err = run(`if $
		p
else if !$
		i`, false)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<i></i>`, t)
	}

	res, err = run(`unless $
		i`, false)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<i></i>`, t)
	}

}

func Test_EmptyAttribute(t *testing.T) {
	res, err := run(`div[name]`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<div name></div>`, t)
	}
}

func Test_RawText(t *testing.T) {
	res, err := run(`html
						script
							var a = 5;
							alert(a)
						style
							body {
								color: white
							}`, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, "<html><script>var a = 5;\nalert(a)</script><style>body {\n\tcolor: white\n}</style></html>", t)
	}
}

func Test_Empty(t *testing.T) {
	res, err := run(``, nil)

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, ``, t)
	}
}

func Test_ArithmeticExpression(t *testing.T) {
	res, err := run(`#{A + B * C}`, map[string]int{"A": 2, "B": 3, "C": 4})

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `14`, t)
	}
}

func Test_BooleanExpression(t *testing.T) {
	res, err := run(`#{C - A < B}`, map[string]int{"A": 2, "B": 3, "C": 4})

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `true`, t)
	}
}

func Test_FuncCall(t *testing.T) {
	res, err := run(`div[data-map=json($)]`, map[string]int{"A": 2, "B": 3, "C": 4})

	if err != nil {
		t.Fatal(err.Error())
	} else {
		expect(res, `<div data-map="{&#34;A&#34;:2,&#34;B&#34;:3,&#34;C&#34;:4}"></div>`, t)
	}
}

func Failing_Test_CompileDir(t *testing.T) {
	tmpl, err := CompileDir("samples/", DefaultDirOptions, DefaultOptions)

	// Test Compilation
	if err != nil {
		t.Fatal(err.Error())
	}

	// Make sure files are added to map correctly
	val1, ok := tmpl["basic"]
	if ok != true || val1 == nil {
		t.Fatal("CompileDir, template not found.")
	}
	val2, ok := tmpl["inherit"]
	if ok != true || val2 == nil {
		t.Fatal("CompileDir, template not found.")
	}
	val3, ok := tmpl["compiledir_test/basic"]
	if ok != true || val3 == nil {
		t.Fatal("CompileDir, template not found.")
	}
	val4, ok := tmpl["compiledir_test/compiledir_test/basic"]
	if ok != true || val4 == nil {
		t.Fatal("CompileDir, template not found.")
	}

	// Make sure file parsing is the same
	var doc1, doc2 bytes.Buffer
	val1.Execute(&doc1, nil)
	val4.Execute(&doc2, nil)
	expect(doc1.String(), doc2.String(), t)

	// Check against CompileFile
	compilefile, err := CompileFile("samples/basic.amber", DefaultOptions)
	if err != nil {
		t.Fatal(err.Error())
	}
	var doc3 bytes.Buffer
	compilefile.Execute(&doc3, nil)
	expect(doc1.String(), doc3.String(), t)
	expect(doc2.String(), doc3.String(), t)

}

func Benchmark_Parse(b *testing.B) {
	code := `
	!!! 5
	html
		head
			title Test Title
		body
			nav#mainNav[data-foo="bar"]
			div#content
				div.left
				div.center
					block center
						p Main Content
							.long ? somevar && someothervar
				div.right`

	for i := 0; i < b.N; i++ {
		cmp := New()
		cmp.Parse(code)
	}
}

func Benchmark_Compile(b *testing.B) {
	b.StopTimer()

	code := `
	!!! 5
	html
		head
			title Test Title
		body
			nav#mainNav[data-foo="bar"]
			div#content
				div.left
				div.center
					block center
						p Main Content
							.long ? somevar && someothervar
				div.right`

	cmp := New()
	cmp.Parse(code)

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		cmp.CompileString()
	}
}

func expect(cur, expected string, t *testing.T) {
	if cur != expected {
		t.Fatalf("Expected {%s} got {%s}.", expected, cur)
	}
}

func expect2(cur, expected string, expected2 string, t *testing.T) {
	if cur != expected && cur != expected2 {
		t.Fatalf("Expected {%s} or {%s} got {%s}.", expected, expected2, cur)
	}
}


func expectWithSource(cur, expected, source string, t *testing.T) {
    if cur != expected {
        t.Fatalf("Expected {%s} got {%s}. Compiled template: {%s}", expected, cur, source)
    }
}

func run(tpl string, data interface{}) (string, error) {
	t, err := Compile(tpl, Options{
        PrettyPrint: false,
        LineNumbers: false,
    })

	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), nil
}

func runPretty(file string, tpl string, data interface{}) (string, string, error) {
    cmp := New()
    cmp.filename = file
    cmp.Parse(tpl)

    cmp.Options = Options{
        PrettyPrint: true,
        LineNumbers: false,
        Funcs: customFuncs,
    }

    t, err := cmp.Compile()
    if err != nil {
        return "", "", err
    }

    compiledTpl, _ := cmp.CompileString()

    var buf bytes.Buffer
    if err = t.Execute(&buf, data); err != nil {
        return "", compiledTpl, err
    }

    return strings.TrimSpace(buf.String()), compiledTpl, nil
}

