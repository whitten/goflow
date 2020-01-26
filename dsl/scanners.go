package dsl

import (
	"bytes"
	"regexp"
	"strings"
)

// Scanner is a unified structure for scanner components which defines their port signature
type Scanner struct {
	// Set is an IIP that contains valid characters. Supports special characters: \r, \n, \t.
	// A regular expression character class can be passed like: "[a-zA-Z\s]".
	Set <-chan string
	// Type is an IIP that contains string token type associated with the set
	Type <-chan string
	// In is an incoming empty token
	In <-chan Token
	// Hit is a successfully matched token
	Hit chan<- Token
	// Miss is the unmodified empty token in case there was no match
	Miss chan<- Token
}

// scanner is used to test Scanner components via common interface
type scanner interface {
	assign(Scanner)
	Process()
}

func (s *Scanner) assign(ports Scanner) {
	s.Set = ports.Set
	s.Type = ports.Type
	s.In = ports.In
	s.Hit = ports.Hit
	s.Miss = ports.Miss
}

// ScanChars scans a token of characters belonging to Set
type ScanChars struct {
	Scanner
}

// Process reads IIPs and validates incoming tokens
func (s *ScanChars) Process() {
	// Read IIPs first
	set := ""
	tokenType := ""
	ok := true
	if set, ok = <-s.Set; !ok {
		return
	}
	matcher := s.matcher(set)

	if tokenType, ok = <-s.Type; !ok {
		return
	}
	// Then process the incoming tokens
	for tok := range s.In {
		buf := bytes.NewBufferString("")
		dataLen := len(tok.File.Data)
		// Read as many chars within the set as possible
		for i := tok.Pos; i < dataLen; i++ {
			r := rune(tok.File.Data[i])
			if r == rune(0) || !matcher(r) {
				break
			}
			buf.WriteRune(r)
		}
		tok.Value = buf.String()
		if buf.Len() > 0 {
			tok.Type = TokenType(tokenType)
			s.Hit <- tok
		} else {
			s.Miss <- tok
		}
	}
}

func (s *ScanChars) matcher(set string) func(r rune) bool {
	if set[0] == '[' && set[len(set)-1] == ']' {
		// A regexp class
		var reg *regexp.Regexp
		var err error
		reg, err = regexp.Compile(set)
		if err != nil {
			// TODO error handling
			return func(r rune) bool {
				return false
			}
		}
		return func(r rune) bool {
			return reg.Match([]byte{byte(r)})
		}
	}
	// Replace special chars
	set = strings.ReplaceAll(set, `\t`, "\t")
	set = strings.ReplaceAll(set, `\r`, "\r")
	set = strings.ReplaceAll(set, `\n`, "\n")
	return func(r rune) bool {
		return strings.ContainsRune(set, r)
	}
}

// ScanKeyword scans a case-insensitive keyword that is not part of another word.
// If Set is an identifier, it makes sure the keyword is not a substring of another identifier.
// If Set is an operator, it makes sure the operator is followed by identifier or space.
type ScanKeyword struct {
	Scanner
}

// Process reads IIPs and validates incoming tokens
func (s *ScanKeyword) Process() {
	// Read IIPs
	word := ""
	tokenType := ""
	ok := true
	if word, ok = <-s.Set; !ok {
		return
	}
	word = strings.ToUpper(word)
	wordLen := len(word)
	if tokenType, ok = <-s.Type; !ok {
		return
	}

	identReg := regexp.MustCompile(`[\w_]`)
	shouldNotBeFollowedBy := identReg
	isIdent := identReg.MatchString(word)
	if !isIdent {
		shouldNotBeFollowedBy = regexp.MustCompile(`[^\w\s]`)
	}

	// Process incoming tokens
	for tok := range s.In {
		dataLen := len(tok.File.Data)
		if tok.Pos+wordLen > dataLen {
			// Data is too short
			s.Miss <- tok
			continue
		}
		tok.Value = string(tok.File.Data[tok.Pos : tok.Pos+wordLen])
		if strings.ToUpper(tok.Value) == word {
			// Potential match, should be followed by EOF or non-word character
			if tok.Pos+wordLen < dataLen {
				nextChar := tok.File.Data[tok.Pos+wordLen]
				if shouldNotBeFollowedBy.Match([]byte{nextChar}) {
					// This is not the whole word
					s.Miss <- tok
					continue
				}
			}
			// Checks passed, it's a match
			tok.Type = TokenType(tokenType)
			tok.Value = word
			s.Hit <- tok
			continue
		}
		// No match
		s.Miss <- tok
	}
}

// ScanComment scans a comment from hash till the end of line
type ScanComment struct {
	Scanner
}

// Process reads IIPs and validates incoming tokens
func (s *ScanComment) Process() {
	// Read IIPs
	prefix := ""
	tokenType := ""
	ok := true
	if prefix, ok = <-s.Set; !ok {
		return
	}
	if tokenType, ok = <-s.Type; !ok {
		return
	}

	// Process incoming tokens
	for tok := range s.In {
		if tok.File.Data[tok.Pos] != prefix[0] {
			s.Miss <- tok
			continue
		}
		buf := bytes.NewBufferString("")
		dataLen := len(tok.File.Data)
		// Read all characters till the end of the line
		for i := tok.Pos; i < dataLen; i++ {
			r := rune(tok.File.Data[i])
			if r == rune(0) || r == rune('\n') || r == rune('\r') {
				break
			}
			buf.WriteRune(r)
		}
		tok.Value = buf.String()
		tok.Type = TokenType(tokenType)
		s.Hit <- tok
	}
}

// ScanQuoted scans a quoted string
type ScanQuoted struct {
	// In is an incoming empty token
	In <-chan Token
	// Hit is a successfully matched token
	Hit chan<- Token
	// Miss is the unmodified empty token in case there was no match
	Miss chan<- Token
}

// ScanInvalid returns an illegal token
type ScanInvalid struct {
	// In is an incoming empty token
	In <-chan Token
	// Token returns and invalid token
	Token chan<- Token
}