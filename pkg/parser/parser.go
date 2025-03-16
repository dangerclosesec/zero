package parser

import (
	"fmt"
	"io"
	"strings"
)

// TokenType identifies the type of lexical tokens
type TokenType int

const (
	ILLEGAL TokenType = iota
	EOF
	IDENT
	STRING
	NUMBER
	LBRACE           // {
	RBRACE           // }
	LPAREN           // (
	RPAREN           // )
	LBRACKET         // [
	RBRACKET         // ]
	ASSIGN           // =
	COMMA            // ,
	WHEN             // when
	DEPENDS_ON       // depends_on
	INCLUDE          // include
	INCLUDE_PLATFORM // include_platform
	VARIABLE         // variable
	TEMPLATE         // template
)

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// Lexer tokenizes input text
type Lexer struct {
	scanner func() Token
	curr    Token
	next    Token
}

// Custom scanner to properly handle comments
type customScanner struct {
	reader    io.Reader
	buffer    []byte
	position  int
	readPos   int
	ch        byte
	line      int
	column    int
	lastToken Token
}

// Initialize a new custom scanner
func newCustomScanner(r io.Reader) *customScanner {
	cs := &customScanner{
		reader: r,
		line:   1,
		column: 0,
	}
	// Read the first character
	cs.readChar()
	return cs
}

// Read the next character from the input
func (cs *customScanner) readChar() {
	if cs.readPos >= len(cs.buffer) {
		// Read more input if needed
		buf := make([]byte, 1024)
		n, err := cs.reader.Read(buf)
		if err != nil || n == 0 {
			cs.ch = 0 // EOF
		} else {
			cs.buffer = buf[:n]
			cs.position = 0
			cs.readPos = 1
			cs.ch = cs.buffer[0]
		}
	} else {
		cs.ch = cs.buffer[cs.readPos]
		cs.position = cs.readPos
		cs.readPos++
	}

	// Update line and column
	if cs.ch == '\n' {
		cs.line++
		cs.column = 0
	} else {
		cs.column++
	}
}

// Peek at the next character
func (cs *customScanner) peekChar() byte {
	if cs.readPos >= len(cs.buffer) {
		return 0
	}
	return cs.buffer[cs.readPos]
}

// Skip whitespace
func (cs *customScanner) skipWhitespace() {
	for cs.ch == ' ' || cs.ch == '\t' || cs.ch == '\n' || cs.ch == '\r' {
		cs.readChar()
	}
}

// Skip comments (both // and #)
func (cs *customScanner) skipComments() bool {
	if cs.ch == '/' && cs.peekChar() == '/' {
		// Skip // comment
		for cs.ch != '\n' && cs.ch != 0 {
			cs.readChar()
		}
		if cs.ch == '\n' {
			cs.readChar() // Skip the newline
		}
		return true
	} else if cs.ch == '#' {
		// Skip # comment
		for cs.ch != '\n' && cs.ch != 0 {
			cs.readChar()
		}
		if cs.ch == '\n' {
			cs.readChar() // Skip the newline
		}
		return true
	}
	return false
}

// Read an identifier
func (cs *customScanner) readIdentifier() string {
	startPosition := cs.position
	for isLetter(cs.ch) || isDigit(cs.ch) || cs.ch == '_' {
		cs.readChar()
	}
	return string(cs.buffer[startPosition:cs.position])
}

// Read a number
func (cs *customScanner) readNumber() string {
	startPosition := cs.position
	for isDigit(cs.ch) || cs.ch == '.' {
		cs.readChar()
	}
	return string(cs.buffer[startPosition:cs.position])
}

// Read a string
func (cs *customScanner) readString() string {
	// Skip the opening quote
	cs.readChar()
	startPosition := cs.position

	for cs.ch != '"' && cs.ch != 0 {
		cs.readChar()
	}

	// Capture the string without the quotes
	result := string(cs.buffer[startPosition:cs.position])

	// Skip the closing quote
	if cs.ch == '"' {
		cs.readChar()
	}

	return result
}

// Scan the next token
func (cs *customScanner) scanToken() Token {
	// Skip whitespace and comments
	cs.skipWhitespace()
	for cs.skipComments() {
		cs.skipWhitespace()
	}

	var tok Token
	tok.Line = cs.line
	tok.Column = cs.column

	switch cs.ch {
	case 0:
		tok.Type = EOF
		tok.Literal = ""
	case '{':
		tok.Type = LBRACE
		tok.Literal = "{"
		cs.readChar()
	case '}':
		tok.Type = RBRACE
		tok.Literal = "}"
		cs.readChar()
	case '(':
		tok.Type = LPAREN
		tok.Literal = "("
		cs.readChar()
	case ')':
		tok.Type = RPAREN
		tok.Literal = ")"
		cs.readChar()
	case '[':
		tok.Type = LBRACKET
		tok.Literal = "["
		cs.readChar()
	case ']':
		tok.Type = RBRACKET
		tok.Literal = "]"
		cs.readChar()
	case '=':
		tok.Type = ASSIGN
		tok.Literal = "="
		cs.readChar()
	case ',':
		tok.Type = COMMA
		tok.Literal = ","
		cs.readChar()
	case '"':
		tok.Type = STRING
		tok.Literal = cs.readString()
	default:
		if isLetter(cs.ch) {
			tok.Literal = cs.readIdentifier()
			// Check for keywords
			switch strings.ToLower(tok.Literal) {
			case "when":
				tok.Type = WHEN
			case "depends_on":
				tok.Type = DEPENDS_ON
			case "include":
				tok.Type = INCLUDE
			case "include_platform":
				tok.Type = INCLUDE_PLATFORM
			case "variable":
				tok.Type = VARIABLE
			case "template":
				tok.Type = TEMPLATE
			default:
				tok.Type = IDENT
			}
		} else if isDigit(cs.ch) {
			tok.Literal = cs.readNumber()
			tok.Type = NUMBER
		} else {
			tok.Type = ILLEGAL
			tok.Literal = string(cs.ch)
			cs.readChar()
		}
	}

	cs.lastToken = tok
	return tok
}

// Helper function to check if a character is a letter
func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z'
}

// Helper function to check if a character is a digit
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

// NewLexer creates a new lexer from a reader
func NewLexer(r io.Reader) *Lexer {
	scanner := newCustomScanner(r)
	l := &Lexer{
		scanner: scanner.scanToken, // Store the scanToken function
	}
	l.next = l.scanToken() // Prime the first token
	l.advance()            // Set curr and next
	return l
}

// Advance moves to the next token
func (l *Lexer) advance() Token {
	prev := l.curr
	l.curr = l.next
	l.next = l.scanToken()
	return prev
}

// Current returns the current token
func (l *Lexer) Current() Token {
	return l.curr
}

// Peek returns the next token without advancing
func (l *Lexer) Peek() Token {
	return l.next
}

// scanToken gets the next token from the scanner
func (l *Lexer) scanToken() Token {
	return l.scanner()
}

// Resource represents a parsed resource block
type Resource struct {
	Type       string
	Name       string
	Attributes map[string]interface{}
	DependsOn  []string
	Conditions map[string][]string
}

// Parser parses our DSL into a resource graph
type Parser struct {
	lexer     *Lexer
	errors    []string
	Resources []Resource
}

// NewParser creates a new parser
func NewParser(r io.Reader) *Parser {
	return &Parser{
		lexer:     NewLexer(r),
		errors:    []string{},
		Resources: []Resource{},
	}
}

// ParseError adds an error to the parser
func (p *Parser) ParseError(format string, args ...interface{}) {
	token := p.lexer.Current()
	errMsg := fmt.Sprintf("Line %d, Column %d: %s", token.Line, token.Column, fmt.Sprintf(format, args...))
	p.errors = append(p.errors, errMsg)
}

// Errors returns all parsing errors
func (p *Parser) Errors() []string {
	return p.errors
}

// Parse parses the entire configuration file
func (p *Parser) Parse() ([]Resource, error) {
	for p.lexer.Current().Type != EOF {
		// Now we look for resource types, includes, or variables
		if p.lexer.Current().Type == IDENT ||
			p.lexer.Current().Type == INCLUDE ||
			p.lexer.Current().Type == INCLUDE_PLATFORM ||
			p.lexer.Current().Type == VARIABLE ||
			p.lexer.Current().Type == TEMPLATE {

			var resourceType string
			switch p.lexer.Current().Type {
			case INCLUDE:
				resourceType = "include"
			case INCLUDE_PLATFORM:
				resourceType = "include_platform"
			case VARIABLE:
				resourceType = "variable"
			case TEMPLATE:
				resourceType = "template"
			default:
				resourceType = p.lexer.Current().Literal
			}

			p.lexer.advance()

			resource, err := p.parseResourceBlock(resourceType)
			if err != nil {
				p.ParseError("Error parsing resource: %v", err)
				p.skipToNextResource()
			} else {
				p.Resources = append(p.Resources, resource)
			}
		} else {
			p.ParseError("Expected resource type identifier, include, or variable statement, got %s", p.lexer.Current().Literal)
			p.lexer.advance()
		}
	}

	if len(p.errors) > 0 {
		return p.Resources, fmt.Errorf("parsing failed with %d errors", len(p.errors))
	}

	return p.Resources, nil
}

// parseResourceBlock parses a resource block
func (p *Parser) parseResourceBlock(resourceType string) (Resource, error) {
	resource := Resource{
		Type:       resourceType,
		Attributes: make(map[string]interface{}),
		Conditions: make(map[string][]string),
	}

	// Parse resource name
	if p.lexer.Current().Type != STRING {
		return resource, fmt.Errorf("expected resource name string, got %s", p.lexer.Current().Literal)
	}
	resource.Name = p.lexer.Current().Literal

	// Special handling for file resources
	if resourceType == "file" {
		// Use the path as given in the resource name
		resource.Attributes["path"] = resource.Name
	}

	// Special handling for include resources
	if resourceType == "include" {
		resource.Attributes["path"] = resource.Name
	}

	// Special handling for variable resources
	if resourceType == "variable" {
		resource.Attributes["name"] = resource.Name
	}

	// Special handling for template resources
	if resourceType == "template" {
		resource.Attributes["name"] = resource.Name
	}

	p.lexer.advance()

	// Parse '{'
	if p.lexer.Current().Type != LBRACE {
		return resource, fmt.Errorf("expected '{', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	// Parse resource attributes until closing '}'
	for p.lexer.Current().Type != RBRACE && p.lexer.Current().Type != EOF {
		attrName := p.lexer.Current().Literal

		switch p.lexer.Current().Type {
		case DEPENDS_ON:
			p.lexer.advance()

			// Parse the new depends_on format: depends_on [ type {"name"} ]
			dependsOn, err := p.parseDependsOn()
			if err != nil {
				return resource, err
			}
			resource.DependsOn = dependsOn

		case WHEN:
			p.lexer.advance()
			if p.lexer.Current().Type != ASSIGN {
				return resource, fmt.Errorf("expected '=' after when, got %s", p.lexer.Current().Literal)
			}
			p.lexer.advance()

			// Parse condition block
			conditions, err := p.parseConditionBlock()
			if err != nil {
				return resource, err
			}
			resource.Conditions = conditions

		case IDENT:
			p.lexer.advance()
			if p.lexer.Current().Type != ASSIGN {
				return resource, fmt.Errorf("expected '=' after attribute name, got %s", p.lexer.Current().Literal)
			}
			p.lexer.advance()

			// Parse attribute value
			var value interface{}
			switch p.lexer.Current().Type {
			case STRING:
				value = p.lexer.Current().Literal
				p.lexer.advance()
			case NUMBER:
				value = p.lexer.Current().Literal // For simplicity, keeping as string
				p.lexer.advance()
			case LBRACKET:
				strArray, err := p.parseStringArray()
				if err != nil {
					return resource, err
				}
				value = strArray
			case LBRACE:
				// Handle nested blocks
				blockMap, err := p.parseBlockMap()
				if err != nil {
					return resource, err
				}
				value = blockMap
			default:
				return resource, fmt.Errorf("unexpected value type for attribute %s: %s",
					attrName, p.lexer.Current().Literal)
			}

			resource.Attributes[attrName] = value
		default:
			return resource, fmt.Errorf("unexpected token in resource block: %s", p.lexer.Current().Literal)
		}
	}

	// Parse closing '}'
	if p.lexer.Current().Type != RBRACE {
		return resource, fmt.Errorf("expected '}', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	return resource, nil
}

// parseDependsOn parses the new depends_on syntax: depends_on [ type {"name"} ]
func (p *Parser) parseDependsOn() ([]string, error) {
	result := []string{}

	// Expect [
	if p.lexer.Current().Type != LBRACKET {
		return result, fmt.Errorf("expected '[' after depends_on, got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	// Parse dependencies until ]
	for p.lexer.Current().Type != RBRACKET && p.lexer.Current().Type != EOF {
		// Get resource type
		if p.lexer.Current().Type != IDENT {
			return result, fmt.Errorf("expected resource type, got %s", p.lexer.Current().Literal)
		}

		resType := p.lexer.Current().Literal
		p.lexer.advance()

		// Expect {
		if p.lexer.Current().Type != LBRACE {
			return result, fmt.Errorf("expected '{' after resource type, got %s", p.lexer.Current().Literal)
		}
		p.lexer.advance()

		// Get resource name (string)
		if p.lexer.Current().Type != STRING {
			return result, fmt.Errorf("expected resource name string, got %s", p.lexer.Current().Literal)
		}

		resName := p.lexer.Current().Literal
		p.lexer.advance()

		// Expect }
		if p.lexer.Current().Type != RBRACE {
			return result, fmt.Errorf("expected '}' after resource name, got %s", p.lexer.Current().Literal)
		}
		p.lexer.advance()

		// Add to result
		dependency := fmt.Sprintf("%s.%s", resType, resName)
		result = append(result, dependency)

		// Check for comma
		if p.lexer.Current().Type == COMMA {
			p.lexer.advance()
		} else if p.lexer.Current().Type != RBRACKET {
			return result, fmt.Errorf("expected ',' or ']', got %s", p.lexer.Current().Literal)
		}
	}

	// Expect ]
	if p.lexer.Current().Type != RBRACKET {
		return result, fmt.Errorf("expected ']', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	return result, nil
}

// parseStringArray parses an array of strings: ["a", "b", "c"]
func (p *Parser) parseStringArray() ([]string, error) {
	result := []string{}

	if p.lexer.Current().Type != LBRACKET {
		return result, fmt.Errorf("expected '[', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	for p.lexer.Current().Type != RBRACKET && p.lexer.Current().Type != EOF {
		if p.lexer.Current().Type != STRING {
			return result, fmt.Errorf("expected string in array, got %s", p.lexer.Current().Literal)
		}

		result = append(result, p.lexer.Current().Literal)
		p.lexer.advance()

		if p.lexer.Current().Type == COMMA {
			p.lexer.advance()
		} else if p.lexer.Current().Type != RBRACKET {
			return result, fmt.Errorf("expected ',' or ']', got %s", p.lexer.Current().Literal)
		}
	}

	if p.lexer.Current().Type != RBRACKET {
		return result, fmt.Errorf("expected ']', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	return result, nil
}

// parseBlockMap parses a block map like: { key1 = "value1", key2 = "value2" }
func (p *Parser) parseBlockMap() (map[string]string, error) {
	result := make(map[string]string)

	if p.lexer.Current().Type != LBRACE {
		return result, fmt.Errorf("expected '{', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	for p.lexer.Current().Type != RBRACE && p.lexer.Current().Type != EOF {
		if p.lexer.Current().Type != IDENT {
			return result, fmt.Errorf("expected identifier in block map, got %s", p.lexer.Current().Literal)
		}

		key := p.lexer.Current().Literal
		p.lexer.advance()

		if p.lexer.Current().Type != ASSIGN {
			return result, fmt.Errorf("expected '=' after key in block map, got %s", p.lexer.Current().Literal)
		}
		p.lexer.advance()

		if p.lexer.Current().Type != STRING {
			return result, fmt.Errorf("expected string value in block map, got %s", p.lexer.Current().Literal)
		}

		result[key] = p.lexer.Current().Literal
		p.lexer.advance()

		if p.lexer.Current().Type == COMMA {
			p.lexer.advance()
		} else if p.lexer.Current().Type != RBRACE {
			return result, fmt.Errorf("expected ',' or '}', got %s", p.lexer.Current().Literal)
		}
	}

	if p.lexer.Current().Type != RBRACE {
		return result, fmt.Errorf("expected '}', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	return result, nil
}

// parseConditionBlock parses a condition block like: { platform = ["linux", "darwin"] }
func (p *Parser) parseConditionBlock() (map[string][]string, error) {
	conditions := make(map[string][]string)

	if p.lexer.Current().Type != LBRACE {
		return conditions, fmt.Errorf("expected '{', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	for p.lexer.Current().Type != RBRACE && p.lexer.Current().Type != EOF {
		if p.lexer.Current().Type != IDENT {
			return conditions, fmt.Errorf("expected condition name, got %s", p.lexer.Current().Literal)
		}

		condName := p.lexer.Current().Literal
		p.lexer.advance()

		if p.lexer.Current().Type != ASSIGN {
			return conditions, fmt.Errorf("expected '=' after condition name, got %s", p.lexer.Current().Literal)
		}
		p.lexer.advance()

		values, err := p.parseStringArray()
		if err != nil {
			return conditions, err
		}

		conditions[condName] = values
	}

	if p.lexer.Current().Type != RBRACE {
		return conditions, fmt.Errorf("expected '}', got %s", p.lexer.Current().Literal)
	}
	p.lexer.advance()

	return conditions, nil
}

// skipToNextResource skips tokens until it finds the next resource block or EOF
func (p *Parser) skipToNextResource() {
	braceDepth := 0

	for p.lexer.Current().Type != EOF {
		if p.lexer.Current().Type == LBRACE {
			braceDepth++
		} else if p.lexer.Current().Type == RBRACE {
			braceDepth--
			if braceDepth <= 0 {
				// We've reached the end of the current resource block
				p.lexer.advance()
				break
			}
		}
		p.lexer.advance()
	}
}
