package parser

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"
)

type Parser struct {
	scanner      *scanner
	filename     string
	currenttoken *token
	namedBlocks  map[string]*NamedBlock
	parent       *Parser
	result       *Block
}

func newParser(rdr io.Reader) *Parser {
	p := new(Parser)
	p.scanner = newScanner(rdr)
	p.namedBlocks = make(map[string]*NamedBlock)
	return p
}

func StringParser(input string) (*Parser, error) {
	return newParser(bytes.NewReader([]byte(input))), nil
}

func FileParser(filename string) (*Parser, error) {
	data, err := ioutil.ReadFile(filename)

	if err != nil {
		return nil, err
	}

	parser := newParser(bytes.NewReader(data))
	parser.filename = filename
	return parser, nil
}

func (p *Parser) Parse() *Block {
	if p.result != nil {
		return p.result
	}

	defer func() {
		if r := recover(); r != nil {
			if rs, ok := r.(string); ok && rs[:len("Jade Error")] == "Jade Error" {
				panic(r)
			}

			pos := p.pos()

			if len(pos.Filename) > 0 {
				panic(fmt.Sprintf("Jade Error in <%s>: %v - Line: %d, Column: %d, Length: %d", pos.Filename, r, pos.LineNum, pos.ColNum, pos.TokenLength))
			} else {
				trace := make([]byte, 1024)
				_ = runtime.Stack(trace, true)
				panic(fmt.Sprintf("Jade Error: %v - Line: %d, Column: %d, Length: %d\n%s", r, pos.LineNum, pos.ColNum, pos.TokenLength, trace))
			}
		}
	}()

	block := newBlock()
	p.advance()

	for {
		if p.currenttoken == nil || p.currenttoken.Kind == tokEOF {
			break
		}

		if p.currenttoken.Kind == tokBlank {
			p.advance()
			continue
		}

		block.push(p.parse())
	}

	if p.parent != nil {
		p.parent.Parse()

		for _, prev := range p.parent.namedBlocks {
			ours := p.namedBlocks[prev.Name]

			if ours == nil {
				continue
			}

			switch ours.Modifier {
			case NamedBlockAppend:
				for i := 0; i < len(ours.Children); i++ {
					prev.push(ours.Children[i])
				}
			case NamedBlockPrepend:
				for i := len(ours.Children) - 1; i >= 0; i-- {
					prev.pushFront(ours.Children[i])
				}
			default:
				prev.Children = ours.Children
			}
		}

		block = p.parent.result
	}

	p.result = block
	return block
}

func (p *Parser) pos() SourcePosition {
	pos := p.scanner.Pos()
	pos.Filename = p.filename
	return pos
}

func (p *Parser) parseRelativeFile(filename string) *Parser {
	if len(p.filename) == 0 {
		panic("Unable to import or extend " + filename + " in a non filesystem based parser.")
	}

	filename = filepath.Join(filepath.Dir(p.filename), filename)

	if strings.IndexRune(filepath.Base(filename), '.') < 0 {
		filename = filename + ".jade"
	}

	parser, err := FileParser(filename)
	if err != nil {
		panic("Unable to read " + filename + ", Error: " + string(err.Error()))
	}

	return parser
}

func tokenKind2Str(token rune) string {
	switch token {
	case tokDoctype:
		return "tokDoctype"
	case tokComment:
		return "tokComment"
	case tokText:
		return "tokText"
	case tokIf:
		return "tokIf"
	case tokElse:
		return "tokElse"
	case tokUnless:
		return "tokUnless"
	case tokEach:
		return "tokEach"
	case tokImport:
		return "tokImport"
	case tokTag:
		return "tokTag"
	case tokBuffered:
		return "tokBuffered"
	case tokAssignment:
		return "tokAssignment"
	case tokNamedBlock:
		return "tokNamedBlock"
	case tokExtends:
		return "tokExtends"
	case tokIndent:
		return "tokIndent"
	case tokMixin:
		return "tokMixin"
	case tokMixinCall:
		return "tokMixinCall"
	}
	return "unknown"
}

func (p *Parser) parse() Node {
	//fmt.Println("token:", tokenKind2Str(p.currenttoken.Kind))

	switch p.currenttoken.Kind {
	case tokDoctype:
		return p.parseDoctype()
	case tokComment:
		return p.parseComment()
	case tokText:
		return p.parseText()
	case tokIf:
		return p.parseIf()
	case tokUnless:
		return p.parseUnless()
	case tokEach:
		return p.parseEach()
	case tokImport:
		return p.parseImport()
	case tokTag:
		return p.parseTag()
	case tokBuffered:
		return p.parseBuffered()
	case tokAssignment:
		return p.parseAssignment()
	case tokNamedBlock:
		return p.parseNamedBlock()
	case tokExtends:
		return p.parseExtends()
	case tokIndent:
		return p.parseBlock(nil)
	case tokMixin:
		return p.parseMixin()
	case tokMixinCall:
		return p.parseMixinCall()
	}

	panic(fmt.Sprintf("Unexpected token: %d", p.currenttoken.Kind))
}

func (p *Parser) expect(typ rune) *token {
	if p.currenttoken.Kind != typ {
		panic("Unexpected token!")
	}
	curtok := p.currenttoken
	p.advance()
	return curtok
}

func (p *Parser) advance() {
	p.currenttoken = p.scanner.Next()
}

func (p *Parser) parseExtends() *Block {
	if p.parent != nil {
		panic("Unable to extend multiple parent templates.")
	}

	tok := p.expect(tokExtends)
	parser := p.parseRelativeFile(tok.Value)
	parser.Parse()
	p.parent = parser
	return newBlock()
}

func (p *Parser) parseBlock(parent Node) *Block {
	p.expect(tokIndent)
	block := newBlock()
	block.SourcePosition = p.pos()

	for {
		if p.currenttoken == nil || p.currenttoken.Kind == tokEOF || p.currenttoken.Kind == tokOutdent {
			break
		}

		if p.currenttoken.Kind == tokBlank {
			p.advance()
			continue
		}

		if p.currenttoken.Kind == tokId ||
			p.currenttoken.Kind == tokClassName ||
			p.currenttoken.Kind == tokAttribute {

			if tag, ok := parent.(*Tag); ok {
				attr := p.expect(p.currenttoken.Kind)
				cond := attr.Data["Condition"]

				switch attr.Kind {
				case tokId:
					tag.Attributes = append(tag.Attributes, Attribute{p.pos(), "id", attr.Value, true, cond})
				case tokClassName:
					tag.Attributes = append(tag.Attributes, Attribute{p.pos(), "class", attr.Value, true, cond})
				case tokAttribute:
					tag.Attributes = append(tag.Attributes, Attribute{p.pos(), attr.Value, attr.Data["Content"], attr.Data["Mode"] == "raw", cond})
				}

				continue
			} else {
				panic("Conditional attributes must be placed immediately within a parent tag.")
			}
		}

		block.push(p.parse())
	}

	p.expect(tokOutdent)

	return block
}

func (p *Parser) parseIf() *Condition {
	tok := p.expect(tokIf)
	cnd := newCondition(tok.Value)
	cnd.SourcePosition = p.pos()

readmore:
	switch p.currenttoken.Kind {
	case tokIndent:
		cnd.Positive = p.parseBlock(cnd)
		goto readmore
	case tokElse:
		p.expect(tokElse)
		if p.currenttoken.Kind == tokIf {
			cnd.Negative = newBlock()
			cnd.Negative.push(p.parseIf())
		} else if p.currenttoken.Kind == tokIndent {
			cnd.Negative = p.parseBlock(cnd)
		} else {
			panic("Unexpected token!")
		}
		goto readmore
	}

	return cnd
}

func (p *Parser) parseUnless() *Condition {
	tok := p.expect(tokUnless)
	cnd := newCondition("!(" + tok.Value + ")")
	cnd.SourcePosition = p.pos()

readmore:
	switch p.currenttoken.Kind {
	case tokIndent:
		cnd.Positive = p.parseBlock(cnd)
		goto readmore
	}

	return cnd
}

func (p *Parser) parseEach() *Each {
	tok := p.expect(tokEach)
	ech := newEach(tok.Value)
	ech.SourcePosition = p.pos()
	ech.X = tok.Data["X"]
	ech.Y = tok.Data["Y"]

	if p.currenttoken.Kind == tokIndent {
		ech.Block = p.parseBlock(ech)
	}

	return ech
}

func (p *Parser) parseImport() *Block {
	tok := p.expect(tokImport)
	node := p.parseRelativeFile(tok.Value).Parse()
	node.SourcePosition = p.pos()
	return node
}

func (p *Parser) parseNamedBlock() *Block {
	tok := p.expect(tokNamedBlock)

	if p.namedBlocks[tok.Value] != nil {
		panic("Multiple definitions of named blocks are not permitted. Block " + tok.Value + " has been re defined.")
	}

	block := newNamedBlock(tok.Value)
	block.SourcePosition = p.pos()

	if tok.Data["Modifier"] == "append" {
		block.Modifier = NamedBlockAppend
	} else if tok.Data["Modifier"] == "prepend" {
		block.Modifier = NamedBlockPrepend
	}

	if p.currenttoken.Kind == tokIndent {
		block.Block = *(p.parseBlock(nil))
	}

	p.namedBlocks[block.Name] = block

	if block.Modifier == NamedBlockDefault {
		return &block.Block
	}

	return newBlock()
}

func (p *Parser) parseDoctype() *Doctype {
	tok := p.expect(tokDoctype)
	node := newDoctype(tok.Value)
	node.SourcePosition = p.pos()
	return node
}

func (p *Parser) parseComment() *Comment {
	tok := p.expect(tokComment)
	cmnt := newComment(tok.Value)
	cmnt.SourcePosition = p.pos()
	cmnt.Silent = tok.Data["Mode"] == "silent"

	if p.currenttoken.Kind == tokIndent {
		cmnt.Block = p.parseBlock(cmnt)
	}

	return cmnt
}

func (p *Parser) parseText() *Text {
	tok := p.expect(tokText)
	node := newText(tok.Value, tok.Data["Mode"] == "raw")
	node.SourcePosition = p.pos()
	return node
}

func (p *Parser) parseBuffered() *Buffered {
	tok := p.expect(tokBuffered)
	node := newBuffered(tok.Value, tok.Data["Mode"] == "escaped")
	node.SourcePosition = p.pos()
	return node
}

func (p *Parser) parseAssignment() *Assignment {
	tok := p.expect(tokAssignment)
	node := newAssignment(tok.Data["X"], tok.Value)
	node.SourcePosition = p.pos()
	return node
}

func (p *Parser) parseTag() *Tag {
	tok := p.expect(tokTag)
	tag := newTag(tok.Value)
	tag.SourcePosition = p.pos()

	ensureBlock := func() {
		if tag.Block == nil {
			tag.Block = newBlock()
		}
	}

readmore:
	switch p.currenttoken.Kind {
	case tokIndent:
		if tag.IsRawText() {
			p.scanner.readRaw = true
		}

		block := p.parseBlock(tag)
		if tag.Block == nil {
			tag.Block = block
		} else {
			for _, c := range block.Children {
				tag.Block.push(c)
			}
		}
	case tokId:
		id := p.expect(tokId)
		if len(id.Data["Condition"]) > 0 {
			panic("Conditional attributes must be placed in a block within a tag.")
		}
		tag.Attributes = append(tag.Attributes, Attribute{p.pos(), "id", id.Value, true, ""})
		goto readmore
	case tokClassName:
		cls := p.expect(tokClassName)
		if len(cls.Data["Condition"]) > 0 {
			panic("Conditional attributes must be placed in a block within a tag.")
		}
		tag.Attributes = append(tag.Attributes, Attribute{p.pos(), "class", cls.Value, true, ""})
		goto readmore
	case tokAttribute:
		attr := p.expect(tokAttribute)
		if len(attr.Data["Condition"]) > 0 {
			panic("Conditional attributes must be placed in a block within a tag.")
		}
		tag.Attributes = append(tag.Attributes, Attribute{p.pos(), attr.Value, attr.Data["Content"], attr.Data["Mode"] == "raw", ""})
		goto readmore
	case tokAttributeList:
		attr := p.expect(tokAttributeList)
		for i := range attr.Children {
			xattr := attr.Children[i]
			tag.Attributes = append(tag.Attributes, Attribute{p.pos(), xattr.Value, xattr.Data["Content"], xattr.Data["Mode"] == "raw", ""})
		}
		goto readmore

	case tokText:
		if p.currenttoken.Data["Mode"] != "piped" {
			ensureBlock()
			tag.Block.pushFront(p.parseText())
			goto readmore
		}

	case tokBuffered:
		ensureBlock()
		tag.Block.pushFront(p.parseBuffered())
		goto readmore

	}

	return tag
}

func (p *Parser) parseMixin() *Mixin {
	tok := p.expect(tokMixin)
	mixin := newMixin(tok.Value, tok.Data["Args"])
	mixin.SourcePosition = p.pos()

	if p.currenttoken.Kind == tokIndent {
		mixin.Block = p.parseBlock(mixin)
	}

	return mixin
}

func (p *Parser) parseMixinCall() *MixinCall {
	tok := p.expect(tokMixinCall)
	mixinCall := newMixinCall(tok.Value, tok.Data["Args"])
	mixinCall.SourcePosition = p.pos()
	return mixinCall
}
