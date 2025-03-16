package parser

import (
	"fmt"
	"io"
	"strings"
	"text/scanner"
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
	scanner scanner.Scanner
	curr    Token
	next    Token
}

// NewLexer creates a new lexer from a reader
func NewLexer(r io.Reader) *Lexer {
	var s scanner.Scanner
	s.Init(r)
	s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanComments

	l := &Lexer{scanner: s}
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

// scanToken scans the next token
func (l *Lexer) scanToken() Token {
	tok := l.scanner.Scan()
	if tok == scanner.EOF {
		return Token{Type: EOF, Literal: "", Line: l.scanner.Line, Column: l.scanner.Column}
	}

	literal := l.scanner.TokenText()
	tokenType := ILLEGAL

	// Determine token type
	switch {
	case tok == scanner.Ident:
		tokenType = IDENT
		// Check for keywords
		switch strings.ToLower(literal) {
		case "when":
			tokenType = WHEN
		case "depends_on":
			tokenType = DEPENDS_ON
		case "include":
			tokenType = INCLUDE
		case "include_platform":
			tokenType = INCLUDE_PLATFORM
		case "variable":
			tokenType = VARIABLE
		case "template":
			tokenType = TEMPLATE
		}
	case tok == scanner.String:
		tokenType = STRING
		// Remove surrounding quotes
		literal = literal[1 : len(literal)-1]
	case tok == scanner.Int || tok == scanner.Float:
		tokenType = NUMBER
	case literal == "{":
		tokenType = LBRACE
	case literal == "}":
		tokenType = RBRACE
	case literal == "(":
		tokenType = LPAREN
	case literal == ")":
		tokenType = RPAREN
	case literal == "[":
		tokenType = LBRACKET
	case literal == "]":
		tokenType = RBRACKET
	case literal == "=":
		tokenType = ASSIGN
	case literal == ",":
		tokenType = COMMA
	}

	return Token{
		Type:    tokenType,
		Literal: literal,
		Line:    l.scanner.Line,
		Column:  l.scanner.Column,
	}
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
