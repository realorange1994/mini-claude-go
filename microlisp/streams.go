package microlisp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// -------- Stream Creation --------

func newFileStream(file *os.File, input, output bool, path string) *Value {
	stream := &LispStream{
		file:     file,
		reader:   bufio.NewReader(file),
		writer:   bufio.NewWriter(file),
		isFile:   true,
		isInput:  input,
		isOutput: output,
		path:     path,
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func newStringInputStream(s string) *Value {
	stream := &LispStream{
		isString:  true,
		isInput:   true,
		strReader: strings.NewReader(s),
		strBuf:    bytes.NewBuffer(nil),
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func newStringOutput() *Value {
	stream := &LispStream{
		isString: true,
		isOutput: true,
		strBuf:   bytes.NewBuffer(nil),
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func (s *LispStream) readChar() (rune, error) {
	if s.isClosed {
		return 0, fmt.Errorf("stream is closed")
	}
	// Synonym stream: resolve the symbol and delegate
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return 0, fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.readChar()
	}
	// Concatenated stream: read from current, advance on EOF
	if s.isConcatenated {
		if s.concatIndex >= len(s.concatStreams) {
			return 0, io.EOF
		}
		for s.concatIndex < len(s.concatStreams) {
			cs := s.concatStreams[s.concatIndex].stream
			r, err := cs.readChar()
			if err == io.EOF {
				s.concatIndex++
				continue
			}
			return r, err
		}
		return 0, io.EOF
	}
	// Two-way stream: read from input
	if s.isTwoWay {
		r, err := s.twoWayInput.stream.readChar()
		if err == nil && s.isEcho {
			// Echo the character to the output stream
			s.twoWayOutput.stream.writeChar(r)
		}
		return r, err
	}
	// Unread buffer takes priority
	if len(s.unreadBuf) > 0 {
		ch := s.unreadBuf[len(s.unreadBuf)-1]
		s.unreadBuf = s.unreadBuf[:len(s.unreadBuf)-1]
		return ch, nil
	}
	// Peeked character: return and clear flag
	if s.hasPeeked {
		s.hasPeeked = false
		return s.peekedChar, nil
	}
	if s.isFile {
		r, _, err := s.reader.ReadRune()
		return r, err
	}
	if s.isString && s.strReader != nil {
		r, _, err := s.strReader.ReadRune()
		return r, err
	}
	return 0, fmt.Errorf("stream is not readable")
}

func (s *LispStream) readLine() (string, error) {
	if s.isClosed {
		return "", fmt.Errorf("stream is closed")
	}
	// Synonym stream: resolve and delegate
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return "", fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.readLine()
	}
	// Concatenated stream: read from current sub-stream
	if s.isConcatenated {
		if s.concatIndex >= len(s.concatStreams) {
			return "", io.EOF
		}
		for s.concatIndex < len(s.concatStreams) {
			cs := s.concatStreams[s.concatIndex].stream
			line, err := cs.readLine()
			if err == io.EOF {
				s.concatIndex++
				if line != "" {
					return line, nil
				}
				continue
			}
			return line, err
		}
		return "", io.EOF
	}
	// Two-way stream: read from input
	if s.isTwoWay {
		return s.twoWayInput.stream.readLine()
	}
	if s.isFile {
		line, err := s.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		// Strip trailing newline
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) == 0 && err == io.EOF {
			return "", io.EOF
		}
		return line, err
	}
	if s.isString && s.strReader != nil {
		var buf strings.Builder
		for {
			r, _, err := s.strReader.ReadRune()
			if err == io.EOF {
				if buf.Len() == 0 {
					return "", io.EOF
				}
				return buf.String(), nil
			}
			if err != nil {
				return "", err
			}
			if r == '\n' {
				return buf.String(), nil
			}
			buf.WriteRune(r)
		}
	}
	return "", fmt.Errorf("stream is not readable")
}

func (s *LispStream) writeChar(c rune) error {
	if s.isClosed {
		return fmt.Errorf("stream is closed")
	}
	// Synonym stream: resolve and delegate
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.writeChar(c)
	}
	// Broadcast stream: write to all targets
	if s.isBroadcast {
		for _, target := range s.broadcastTargets {
			if err := target.stream.writeChar(c); err != nil {
				return err
			}
		}
		return nil
	}
	// Two-way stream: write to output
	if s.isTwoWay {
		return s.twoWayOutput.stream.writeChar(c)
	}
	if s.isFile {
		s.writer.WriteRune(c)
		return nil
	}
	if s.isString && s.strBuf != nil {
		s.strBuf.WriteRune(c)
		return nil
	}
	return fmt.Errorf("stream is not writable")
}

func (s *LispStream) writeString(str string) error {
	if s.isClosed {
		return fmt.Errorf("stream is closed")
	}
	// Synonym stream: resolve and delegate
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.writeString(str)
	}
	// Broadcast stream: write to all targets
	if s.isBroadcast {
		for _, target := range s.broadcastTargets {
			if err := target.stream.writeString(str); err != nil {
				return err
			}
		}
		return nil
	}
	// Two-way stream: write to output
	if s.isTwoWay {
		return s.twoWayOutput.stream.writeString(str)
	}
	if s.isFile {
		s.writer.WriteString(str)
		return nil
	}
	if s.isString && s.strBuf != nil {
		s.strBuf.WriteString(str)
		return nil
	}
	return fmt.Errorf("stream is not writable")
}

func (s *LispStream) writeLine(str string) error {
	if err := s.writeString(str); err != nil {
		return err
	}
	return s.writeChar('\n')
}

func (s *LispStream) flush() error {
	if s.isClosed {
		return nil
	}
	// Synonym stream: flush the resolved stream
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return nil
		}
		return resolved.stream.flush()
	}
	// Broadcast stream: flush all targets
	if s.isBroadcast {
		for _, target := range s.broadcastTargets {
			if err := target.stream.flush(); err != nil {
				return err
			}
		}
		return nil
	}
	// Two-way stream: flush output
	if s.isTwoWay {
		return s.twoWayOutput.stream.flush()
	}
	if s.isFile && s.writer != nil {
		return s.writer.Flush()
	}
	return nil
}

func (s *LispStream) close() error {
	if s.isClosed {
		return nil
	}
	s.isClosed = true
	// Broadcast stream: close all targets
	if s.isBroadcast {
		for _, target := range s.broadcastTargets {
			if target != nil && target.typ == VStream {
				target.stream.close()
			}
		}
		return nil
	}
	// Concatenated stream: close all inputs
	if s.isConcatenated {
		for _, src := range s.concatStreams {
			if src != nil && src.typ == VStream {
				src.stream.close()
			}
		}
		return nil
	}
	// Two-way stream: close input and output
	if s.isTwoWay {
		if s.twoWayInput != nil && s.twoWayInput.typ == VStream {
			s.twoWayInput.stream.close()
		}
		if s.twoWayOutput != nil && s.twoWayOutput.typ == VStream {
			s.twoWayOutput.stream.close()
		}
		return nil
	}
	if s.isFile && s.file != nil {
		if s.writer != nil {
			s.writer.Flush()
		}
		return s.file.Close()
	}
	return nil
}

func (s *LispStream) getStringOutput() string {
	if s.isString && s.strBuf != nil {
		return s.strBuf.String()
	}
	return ""
}

// -------- Stream Builtins --------

func builtinOpen(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("open: need pathname")
	}
	path := primaryValue(args[0]).str
	// Parse keyword arguments
	direction := ":input"
	ifExists := ":error"
	ifNotExist := ":create"
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg.typ == VSym {
			switch arg.str {
			case ":DIRECTION":
				if i+1 < len(args) {
					i++
					direction = primaryValue(args[i]).str
				}
			case ":IF-EXISTS":
				if i+1 < len(args) {
					i++
					ifExists = primaryValue(args[i]).str
				}
			case ":IF-DOES-NOT-EXIST":
				if i+1 < len(args) {
					i++
					ifNotExist = primaryValue(args[i]).str
				}
			}
		}
	}

	// Strip leading : from keyword values and lowercase for case-insensitive matching
	dirStr := strings.ToLower(direction)
	if len(dirStr) > 0 && dirStr[0] == ':' {
		dirStr = dirStr[1:]
	}
	existsStr := strings.ToLower(ifExists)
	if len(existsStr) > 0 && existsStr[0] == ':' {
		existsStr = existsStr[1:]
	}
	notExistStr := strings.ToLower(ifNotExist)
	if len(notExistStr) > 0 && notExistStr[0] == ':' {
		notExistStr = notExistStr[1:]
	}

	var flag int
	var input, output bool
	switch dirStr {
	case "input":
		flag = os.O_RDONLY
		input = true
	case "output":
		if existsStr == "append" {
			flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		} else if existsStr == "supersede" {
			flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		} else {
			flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		output = true
	case "io":
		flag = os.O_RDWR | os.O_CREATE
		input = true
		output = true
	default:
		return nil, fmt.Errorf("open: invalid direction: %s", dirStr)
	}

	if notExistStr == "error" {
		flag |= os.O_EXCL
	}

	file, err := os.OpenFile(path, flag, 0644)
	if err != nil {
		return nil, fmt.Errorf("open: %s", err)
	}
	return newFileStream(file, input, output, path), nil
}

func builtinClose(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("close: need a stream")
	}
	stream := args[0]
	if stream.typ != VStream {
		return nil, fmt.Errorf("close: not a stream")
	}
	err := stream.stream.close()
	if err != nil {
		return vbool(false), nil
	}
	return vbool(true), nil
}

func builtinOpenInputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("open-input-file: need pathname")
	}
	path := primaryValue(args[0]).str
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open-input-file: %s", err)
	}
	return newFileStream(file, true, false, path), nil
}

func builtinOpenOutputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("open-output-file: need pathname")
	}
	path := primaryValue(args[0]).str
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("open-output-file: %s", err)
	}
	return newFileStream(file, false, true, path), nil
}

func builtinStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VStream), nil
}

func builtinStreamInputP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isInput), nil
}

func builtinStreamOutputP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isOutput), nil
}

func builtinOpenStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(!args[0].stream.isClosed), nil
}

func builtinStreamElementType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("stream-element-type: need a stream")
	}
	if args[0].typ != VStream {
		return nil, fmt.Errorf("stream-element-type: not a stream")
	}
	return vsym("CHARACTER"), nil
}

func builtinReadCharNoHang(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-char-no-hang: need a stream")
	}
	stream := args[0]
	if stream.typ != VStream || stream.stream == nil {
		return nil, fmt.Errorf("read-char-no-hang: not a stream")
	}
	s := stream.stream
	if s.isClosed {
		return nil, fmt.Errorf("read-char-no-hang: stream is closed")
	}
	if !s.isInput {
		return nil, fmt.Errorf("read-char-no-hang: not an input stream")
	}
	// Try non-blocking read: check if data is available
	if s.reader != nil {
		if s.reader.Buffered() > 0 {
			r, _, err := s.reader.ReadRune()
			if err != nil {
				return vnil(), nil
			}
			return vchar(r), nil
		}
	}
	if s.strReader != nil {
		if s.strReader.Len() > 0 {
			r, _, err := s.strReader.ReadRune()
			if err != nil {
				return vnil(), nil
			}
			return vchar(r), nil
		}
	}
	// No data available without blocking
	return vnil(), nil
}

func builtinSetSyntaxFromChar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-syntax-from-char: need to-char and from-char")
	}
	toChar := args[0]
	fromChar := args[1]
	if toChar.typ != VChar || fromChar.typ != VChar {
		return nil, fmt.Errorf("set-syntax-from-char: arguments must be characters")
	}
	toR := toChar.ch
	fromR := fromChar.ch
	// Get the current readtable
	rt := currentReadtable
	if rt == nil {
		return nil, fmt.Errorf("set-syntax-from-char: no current readtable")
	}
	// Copy macro function from from-char to to-char in the current readtable
	if rt.macroFns != nil {
		if fn, ok := rt.macroFns[fromR]; ok {
			rt.macroFns[toR] = fn
		} else {
			delete(rt.macroFns, toR)
		}
	}
	return vbool(true), nil
}

func builtinReadPreservingWhitespace(args []*Value) (*Value, error) {
	// read-preserving-whitespace behaves like read for now
	// (the difference is that trailing whitespace is preserved, which
	// requires deeper integration with the reader)
	var readArgs []*Value
	if len(args) > 0 && args[0].typ == VStream {
		readArgs = args
	}
	return builtinRead(readArgs)
}

func builtinFileStringLength(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("file-string-length: need stream and object")
	}
	stream := args[0]
	obj := args[1]
	if stream.typ != VStream || stream.stream == nil {
		return nil, fmt.Errorf("file-string-length: not a stream")
	}
	// Return the length the string representation would have
	str := writeToString(obj)
	return vnum(float64(len(str))), nil
}

func builtinReadChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-char: need a stream")
	}
	stream := args[0]
	if stream.typ != VStream {
		return nil, fmt.Errorf("read-char: not a stream")
	}
	r, err := stream.stream.readChar()
	if err == io.EOF {
		return vsym("eof"), nil
	}
	if err != nil {
		return nil, err
	}
	return vstr(string(r)), nil
}

func builtinReadLine(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-line: need a stream")
	}
	stream := args[0]
	if stream.typ != VStream {
		return nil, fmt.Errorf("read-line: not a stream")
	}
	line, err := stream.stream.readLine()
	if err == io.EOF {
		return list(vstr(line), vbool(false)), nil
	}
	if err != nil {
		return nil, err
	}
	return list(vstr(line), vbool(true)), nil
}

func builtinWriteChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-char: need a character")
	}
	c := primaryValue(args[0])
	cStr := ""
	if c.typ == VStr {
		cStr = c.str
	} else {
		cStr = writeToString(c)
	}

	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-char: not a stream")
		}
	}
	if len(cStr) > 0 {
		if err := stream.stream.writeChar(rune(cStr[0])); err != nil {
			return nil, err
		}
	}
	return vnil(), nil
}

func builtinWriteString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-string: need a string")
	}
	str := primaryValue(args[0])
	strStr := ""
	if str.typ == VStr {
		strStr = str.str
	} else {
		strStr = writeToString(str)
	}

	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-string: not a stream")
		}
	}
	if err := stream.stream.writeString(strStr); err != nil {
		return nil, err
	}
	return vnil(), nil
}

func builtinWriteLine(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-line: need a string")
	}
	str := primaryValue(args[0])
	strStr := ""
	if str.typ == VStr {
		strStr = str.str
	} else {
		strStr = writeToString(str)
	}

	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-line: not a stream")
		}
	}
	if err := stream.stream.writeLine(strStr); err != nil {
		return nil, err
	}
	return vnil(), nil
}

func builtinReadFromString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-from-string: need a string")
	}
	str := primaryValue(args[0])
	strStr := ""
	if str.typ == VStr {
		strStr = str.str
	} else {
		strStr = writeToString(str)
	}

	// Parse keyword arguments
	eofErrorP := true
	eofValue := vnil()
	startPos := 0
	endPos := len([]rune(strStr))

	for i := 1; i < len(args); {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":EOF-ERROR-P":
				if i+1 < len(args) {
					i++
					eofErrorP = !isNil(primaryValue(args[i]))
				}
			case ":EOF-VALUE":
				if i+1 < len(args) {
					i++
					eofValue = primaryValue(args[i])
				}
			case ":START":
				if i+1 < len(args) {
					i++
					startPos = int(toNum(primaryValue(args[i])))
				}
			case ":END":
				if i+1 < len(args) {
					i++
					endPos = int(toNum(primaryValue(args[i])))
				}
			case ":PRESERVE-WHITESPACE":
				if i+1 < len(args) {
					i++
					// preserveWhitespace not yet fully implemented
				}
			default:
				i++
				if i < len(args) && args[i-1].typ == VSym && args[i-1].str[0] == ':' {
					i++
				}
			}
		} else {
			i++
		}
	}

	// Apply start/end
	runes := []rune(strStr)
	if startPos < 0 {
		startPos = 0
	}
	if startPos > len(runes) {
		startPos = len(runes)
	}
	if endPos > len(runes) {
		endPos = len(runes)
	}
	if startPos > endPos {
		startPos = endPos
	}

	subStr := string(runes[startPos:endPos])

	v, pos, err := parseExprWithPos(subStr)
	if err != nil {
		if !eofErrorP {
			return multiVal(eofValue, vnum(float64(startPos))), nil
		}
		return nil, err
	}

	// Return position relative to original string
	return multiVal(v, vnum(float64(startPos+pos))), nil
}

func builtinPrint(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnil(), nil
	}
	stream := stdoutStream
	objEnd := len(args)
	if len(args) > 1 {
		last := args[len(args)-1]
		if last.typ == VStream {
			stream = last
			objEnd = len(args) - 1
		}
	}
	for i := 0; i < objEnd; i++ {
		if err := stream.stream.writeString(writeToString(primaryValue(args[i]))); err != nil {
			return nil, err
		}
	}
	if err := stream.stream.writeChar('\n'); err != nil {
		return nil, err
	}
	stream.stream.flush()
	// CL: print returns the object printed
	return primaryValue(args[0]), nil
}


func builtinPrincl(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("princ: need an object")
	}
	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("princ: not a stream")
		}
	}
	if err := stream.stream.writeString(princToString(primaryValue(args[0]))); err != nil {
		return nil, err
	}
	// CL: princ returns the object printed
	return primaryValue(args[0]), nil
}

func builtinPrin1(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("prin1: need an object")
	}
	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("prin1: not a stream")
		}
	}
	if err := stream.stream.writeString(writeToString(primaryValue(args[0]))); err != nil {
		return nil, err
	}
	// CL: prin1 returns the object printed
	return primaryValue(args[0]), nil
}

// builtinWrite: ANSI CL write function with keyword args
// (write object &key stream escape radix base circle pretty level length case array gensym readably)
func builtinWrite(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write: need an object")
	}
	obj := primaryValue(args[0])
	stream := stdoutStream

	// Parse keyword arguments
	escape := true // default for write: *print-escape* which defaults to t
	for i := 1; i < len(args)-1; i += 2 {
		kw := args[i]
		if kw.typ == VSym {
			switch strings.ToUpper(kw.str) {
			case "STREAM":
				if args[i+1].typ == VStream {
					stream = args[i+1]
				}
			case "ESCAPE":
				escape = !isNil(args[i+1])
			}
		}
	}

	var output string
	if escape {
		output = writeToString(obj)
	} else {
		output = princToString(obj)
	}
	if err := stream.stream.writeString(output); err != nil {
		return nil, err
	}
	stream.stream.flush()
	// CL: write returns the object
	return obj, nil
}

func builtinFinishOutput(args []*Value) (*Value, error) {
	stream := stdoutStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("finish-output: not a stream")
		}
	}
	if err := stream.stream.flush(); err != nil {
		return nil, err
	}
	return vnil(), nil
}

func builtinTerpri(args []*Value) (*Value, error) {
	stream := stdoutStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("terpri: not a stream")
		}
	}
	if err := stream.stream.writeChar('\n'); err != nil {
		return nil, err
	}
	return vnil(), nil
}

func builtinFreshLine(args []*Value) (*Value, error) {
	stream := stdoutStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("fresh-line: not a stream")
		}
	}
	if err := stream.stream.writeChar('\n'); err != nil {
		return nil, err
	}
	return vnil(), nil
}

func builtinGetStringOutput(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("get-output-stream-string: need a stream")
	}
	stream := args[0]
	if stream.typ != VStream {
		return nil, fmt.Errorf("get-output-stream-string: not a stream")
	}
	return vstr(stream.stream.getStringOutput()), nil
}

func builtinMakeStringInputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-string-input-stream: need a string")
	}
	strVal := primaryValue(args[0])
	strStr := ""
	if strVal.typ == VStr {
		strStr = strVal.str
	} else {
		strStr = writeToString(strVal)
	}
	return newStringInputStream(strStr), nil
}

func builtinMakeStringOutputStream(args []*Value) (*Value, error) {
	_ = args
	return newStringOutput(), nil
}

func builtinListen(args []*Value) (*Value, error) {
	stream := stdinStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("listen: not a stream")
		}
	}
	// For string-input-stream, check if there's remaining data
	if stream.stream.isString && stream.stream.strReader != nil {
		// strings.Reader.Len() returns remaining bytes
		// We can't check length directly, but we can try to read 0 bytes
		return vbool(stream.stream.strReader.Len() > 0), nil
	}
	// For buffered readers, try a non-blocking Peek
	if stream.stream.reader != nil {
		r := stream.stream.reader
		_, err := r.Peek(1)
		if err != nil {
			return vbool(false), nil
		}
		return vbool(true), nil
	}
	return vbool(false), nil
}

func builtinClearInput(args []*Value) (*Value, error) {
	stream := stdinStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("clear-input: not a stream")
		}
	}
	if stream == stdinStream {
		// Discard buffered stdin
		reader := bufio.NewReader(os.Stdin)
		for {
			ch, err := reader.ReadByte()
			if err != nil {
				break
			}
			if ch == '\n' {
				break
			}
		}
	}
	return vnil(), nil
}

func builtinForceOutput(args []*Value) (*Value, error) {
	stream := stdoutStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("force-output: not a stream")
		}
	}
	stream.stream.flush()
	return vnil(), nil
}

func builtinClearOutput(args []*Value) (*Value, error) {
	// clear-output is typically a no-op in buffered contexts
	// For string-output-stream, discard buffer
	stream := stdoutStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return nil, fmt.Errorf("clear-output: not a stream")
		}
	}
	if stream.stream.isString && stream.stream.strBuf != nil {
		stream.stream.strBuf.Reset()
	}
	return vnil(), nil
}

func builtinYOrNP(args []*Value) (*Value, error) {
	prompt := " (y or n) "
	if len(args) > 0 && args[0].typ == VStr {
		prompt = args[0].str
	}
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return vbool(false), nil
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "y" || line == "yes" {
			return vbool(true), nil
		}
		if line == "n" || line == "no" {
			return vbool(false), nil
		}
		fmt.Print("Please answer y or n: ")
	}
}

func builtinYesOrNoP(args []*Value) (*Value, error) {
	prompt := " (yes or no) "
	if len(args) > 0 && args[0].typ == VStr {
		prompt = args[0].str
	}
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return vbool(false), nil
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "yes" || line == "y" {
			return vbool(true), nil
		}
		if line == "no" || line == "n" {
			return vbool(false), nil
		}
		fmt.Print("Please answer yes or no: ")
	}
}

// -------- Composite Stream Constructors --------

// resolveStreamBySymbol resolves a symbol name to a stream Value.
func resolveStreamBySymbol(symName string) *Value {
	v, err := globalEnv.Get(symName)
	if err != nil || v == nil {
		return nil
	}
	if v.typ == VStream {
		return v
	}
	return nil
}

func newSynonymStream(symName string) *Value {
	stream := &LispStream{
		isSynonym: true,
		synSym:    symName,
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func newBroadcastStream(streams []*Value) *Value {
	stream := &LispStream{
		isBroadcast:      true,
		isOutput:         true,
		broadcastTargets: streams,
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func newConcatenatedStream(streams []*Value) *Value {
	stream := &LispStream{
		isConcatenated: true,
		isInput:        true,
		concatStreams:  streams,
		concatIndex:    0,
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func newEchoStream(input, output *Value) *Value {
	// Echo stream: reads from input, echoes to output
	// We use two-way stream with echo mode
	stream := &LispStream{
		isTwoWay:     true,
		isInput:      true,
		isOutput:     true,
		isEcho:       true,
		twoWayInput:  input,
		twoWayOutput: output,
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

func newTwoWayStream(input, output *Value) *Value {
	stream := &LispStream{
		isTwoWay:     true,
		isInput:      true,
		isOutput:     true,
		twoWayInput:  input,
		twoWayOutput: output,
	}
	v := gcv()
	v.typ = VStream
	v.stream = stream
	return v
}

// -------- Composite Stream Builtins --------

func builtinMakeSynonymStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-synonym-stream: need a symbol")
	}
	sym := primaryValue(args[0])
	if sym.typ != VSym {
		return nil, fmt.Errorf("make-synonym-stream: expected a symbol")
	}
	return newSynonymStream(sym.str), nil
}

func builtinMakeBroadcastStream(args []*Value) (*Value, error) {
	// CL: (make-broadcast-stream &rest streams) — 0 or more args
	streams := []*Value{}
	for _, arg := range args {
		if arg.typ != VStream {
			return nil, fmt.Errorf("make-broadcast-stream: expected a stream")
		}
		streams = append(streams, arg)
	}
	return newBroadcastStream(streams), nil
}

func builtinMakeConcatenatedStream(args []*Value) (*Value, error) {
	// CL: (make-concatenated-stream &rest streams) — 1 or more args
	if len(args) < 1 {
		return nil, fmt.Errorf("make-concatenated-stream: need at least one stream")
	}
	streams := []*Value{}
	for _, arg := range args {
		if arg.typ != VStream {
			return nil, fmt.Errorf("make-concatenated-stream: expected a stream")
		}
		streams = append(streams, arg)
	}
	return newConcatenatedStream(streams), nil
}

func builtinMakeTwoWayStream(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("make-two-way-stream: need input and output streams")
	}
	input := args[0]
	output := args[1]
	if input.typ != VStream {
		return nil, fmt.Errorf("make-two-way-stream: expected input stream")
	}
	if output.typ != VStream {
		return nil, fmt.Errorf("make-two-way-stream: expected output stream")
	}
	return newTwoWayStream(input, output), nil
}

func builtinMakeEchoStream(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("make-echo-stream: need input and output streams")
	}
	input := args[0]
	output := args[1]
	if input.typ != VStream {
		return nil, fmt.Errorf("make-echo-stream: expected input stream")
	}
	if output.typ != VStream {
		return nil, fmt.Errorf("make-echo-stream: expected output stream")
	}
	return newEchoStream(input, output), nil
}

func builtinSynonymStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isSynonym), nil
}

func builtinBroadcastStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isBroadcast), nil
}

func builtinConcatenatedStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isConcatenated), nil
}

func builtinTwoWayStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isTwoWay), nil
}

func builtinEchoStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isEcho), nil
}

func builtinStringStreamP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ != VStream {
		return vbool(false), nil
	}
	return vbool(args[0].stream.isString), nil
}

func builtinEchoStreamInputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isEcho {
		return nil, fmt.Errorf("echo-stream-input-stream: expected an echo stream")
	}
	return args[0].stream.twoWayInput, nil
}

func builtinEchoStreamOutputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isEcho {
		return nil, fmt.Errorf("echo-stream-output-stream: expected an echo stream")
	}
	return args[0].stream.twoWayOutput, nil
}

func builtinSynonymStreamSymbol(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isSynonym {
		return nil, fmt.Errorf("synonym-stream-symbol: expected a synonym stream")
	}
	return vsym(args[0].stream.synSym), nil
}

func builtinBroadcastStreamStreams(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isBroadcast {
		return nil, fmt.Errorf("broadcast-stream-streams: expected a broadcast stream")
	}
	result := vnil()
	// Reverse to get correct order
	for i := len(args[0].stream.broadcastTargets) - 1; i >= 0; i-- {
		result = cons(args[0].stream.broadcastTargets[i], result)
	}
	return result, nil
}

func builtinConcatenatedStreamStreams(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isConcatenated {
		return nil, fmt.Errorf("concatenated-stream-streams: expected a concatenated stream")
	}
	result := vnil()
	for i := len(args[0].stream.concatStreams) - 1; i >= 0; i-- {
		result = cons(args[0].stream.concatStreams[i], result)
	}
	return result, nil
}

func builtinTwoWayStreamInputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isTwoWay {
		return nil, fmt.Errorf("two-way-stream-input-stream: expected a two-way stream")
	}
	return args[0].stream.twoWayInput, nil
}

func builtinTwoWayStreamOutputStream(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0].typ != VStream || !args[0].stream.isTwoWay {
		return nil, fmt.Errorf("two-way-stream-output-stream: expected a two-way stream")
	}
	return args[0].stream.twoWayOutput, nil
}

// -------- Peek / Unread / Byte I/O --------

// builtinPeekChar implements peek-char.
// (peek-char) or (peek-char nil) — skip whitespace, peek next non-whitespace
// (peek-char t) — same as nil (skip whitespace)
// (peek-char char) — skip until char, peek char
// Returns the peeked character (not consumed).
func builtinPeekChar(args []*Value) (*Value, error) {
	stream := stdinStream
	echoP := vnil()
	if len(args) > 0 {
		echoP = primaryValue(args[0])
	}
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("peek-char: not a stream")
		}
	}

	for {
		// Check unread buffer
		if len(stream.stream.unreadBuf) > 0 {
			ch := stream.stream.unreadBuf[len(stream.stream.unreadBuf)-1]
			// If echoP is nil/t (whitespace skip mode), skip whitespace
			if echoP.typ == VNil || (echoP.typ == VSym && echoP.str == "t") {
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
					// consume and continue
					stream.stream.unreadBuf = stream.stream.unreadBuf[:len(stream.stream.unreadBuf)-1]
					continue
				}
			}
			// If echoP is a character, check if it matches
			if echoP.typ == VChar {
				if ch != echoP.ch {
					// Not the target, consume and continue
					stream.stream.unreadBuf = stream.stream.unreadBuf[:len(stream.stream.unreadBuf)-1]
					continue
				}
			}
			return vstr(string(ch)), nil
		}
		// Check peeked char
		if stream.stream.hasPeeked {
			ch := stream.stream.peekedChar
			if echoP.typ == VNil || (echoP.typ == VSym && echoP.str == "t") {
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
					stream.stream.hasPeeked = false
					continue
				}
			}
			if echoP.typ == VChar {
				if ch != echoP.ch {
					stream.stream.hasPeeked = false
					continue
				}
			}
			return vstr(string(ch)), nil
		}
		// Read from underlying stream
		r, err := stream.stream.readUnderlying()
		if err == io.EOF {
			return vsym("eof"), nil
		}
		if err != nil {
			return nil, err
		}
		if echoP.typ == VNil || (echoP.typ == VSym && echoP.str == "t") {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				continue
			}
		}
		if echoP.typ == VChar {
			if r != echoP.ch {
				continue
			}
		}
		// Cache as peeked
		stream.stream.peekedChar = r
		stream.stream.hasPeeked = true
		return vstr(string(r)), nil
	}
}

// readUnderlying reads from the underlying stream, bypassing peek/unread buffers.
func (s *LispStream) readUnderlying() (rune, error) {
	if s.isClosed {
		return 0, fmt.Errorf("stream is closed")
	}
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return 0, fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.readUnderlying()
	}
	if s.isConcatenated {
		if s.concatIndex >= len(s.concatStreams) {
			return 0, io.EOF
		}
		for s.concatIndex < len(s.concatStreams) {
			cs := s.concatStreams[s.concatIndex].stream
			r, err := cs.readUnderlying()
			if err == io.EOF {
				s.concatIndex++
				continue
			}
			return r, err
		}
		return 0, io.EOF
	}
	if s.isTwoWay {
		return s.twoWayInput.stream.readUnderlying()
	}
	if s.isFile {
		r, _, err := s.reader.ReadRune()
		return r, err
	}
	if s.isString && s.strReader != nil {
		r, _, err := s.strReader.ReadRune()
		return r, err
	}
	return 0, fmt.Errorf("stream is not readable")
}

// builtinUnreadChar implements unread-char.
// (unread-char char &optional input-stream)
func builtinUnreadChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("unread-char: need a character")
	}
	ch := primaryValue(args[0])
	chStr := ""
	if ch.typ == VStr {
		chStr = ch.str
	} else if ch.typ == VChar {
		chStr = string(ch.ch)
	} else {
		return nil, fmt.Errorf("unread-char: not a character")
	}
	if len(chStr) == 0 {
		return nil, fmt.Errorf("unread-char: empty character")
	}

	stream := stdinStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("unread-char: not a stream")
		}
	}

	// Push onto unread buffer
	stream.stream.unreadBuf = append(stream.stream.unreadBuf, rune(chStr[0]))
	return vnil(), nil
}

// readByte reads a single byte from the stream.
func (s *LispStream) readByte() (byte, error) {
	if s.isClosed {
		return 0, fmt.Errorf("stream is closed")
	}
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return 0, fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.readByte()
	}
	if s.isConcatenated {
		if s.concatIndex >= len(s.concatStreams) {
			return 0, io.EOF
		}
		for s.concatIndex < len(s.concatStreams) {
			cs := s.concatStreams[s.concatIndex].stream
			b, err := cs.readByte()
			if err == io.EOF {
				s.concatIndex++
				continue
			}
			return b, err
		}
		return 0, io.EOF
	}
	if s.isTwoWay {
		return s.twoWayInput.stream.readByte()
	}
	// Handle unreadBuf — pop single-byte ASCII chars
	if len(s.unreadBuf) > 0 {
		ch := s.unreadBuf[len(s.unreadBuf)-1]
		s.unreadBuf = s.unreadBuf[:len(s.unreadBuf)-1]
		if ch < 128 {
			return byte(ch), nil
		}
		// Non-ASCII: can't represent as single byte, fall through
	}
	if s.isFile {
		return s.reader.ReadByte()
	}
	if s.isString && s.strReader != nil {
		b, err := s.strReader.ReadByte()
		return b, err
	}
	return 0, fmt.Errorf("stream is not readable")
}

// writeByte writes a single byte to the stream.
func (s *LispStream) writeByte(b byte) error {
	if s.isClosed {
		return fmt.Errorf("stream is closed")
	}
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved == nil {
			return fmt.Errorf("synonym stream: unbound symbol %s", s.synSym)
		}
		return resolved.stream.writeByte(b)
	}
	if s.isBroadcast {
		for _, target := range s.broadcastTargets {
			if err := target.stream.writeByte(b); err != nil {
				return err
			}
		}
		return nil
	}
	if s.isTwoWay {
		return s.twoWayOutput.stream.writeByte(b)
	}
	if s.isFile {
		return s.writer.WriteByte(b)
	}
	if s.isString && s.strBuf != nil {
		s.strBuf.WriteByte(b)
		return nil
	}
	return fmt.Errorf("stream is not writable")
}

// builtinReadByte implements read-byte.
// (read-byte binary-input-stream &optional eof-error-p eof-value)
func builtinReadByte(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-byte: need a stream")
	}
	stream := args[0]
	if stream.typ != VStream {
		return nil, fmt.Errorf("read-byte: not a stream")
	}
	b, err := stream.stream.readByte()
	if err == io.EOF {
		eofVal := vnil()
		if len(args) > 2 {
			eofVal = args[2]
		}
		return eofVal, nil
	}
	if err != nil {
		return nil, err
	}
	return vnum(float64(b)), nil
}

// builtinWriteByte implements write-byte.
// (write-byte integer binary-output-stream)
func builtinWriteByte(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-byte: need an integer")
	}
	b := int(primaryValue(args[0]).num)
	if b < 0 || b > 255 {
		return nil, fmt.Errorf("write-byte: integer must be 0-255")
	}

	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-byte: not a stream")
		}
	}
	if err := stream.stream.writeByte(byte(b)); err != nil {
		return nil, err
	}
	return vnil(), nil
}

// builtinInteractiveStreamP implements interactive-stream-p.
// Returns true for stdin, stdout, stderr streams.
func builtinInteractiveStreamP(args []*Value) (*Value, error) {
	stream := stdinStream
	if len(args) > 0 {
		stream = args[0]
		if stream.typ != VStream {
			return vbool(false), nil
		}
	}
	s := stream.stream
	// A stream is interactive if it's a file stream bound to stdin/stdout/stderr
	// or if it's a synonym stream pointing to one
	if s.isSynonym {
		resolved := resolveStreamBySymbol(s.synSym)
		if resolved != nil {
			return builtinInteractiveStreamP([]*Value{resolved})
		}
		return vbool(false), nil
	}
	isInteractive := s.isFile && (s.file == os.Stdin || s.file == os.Stdout || s.file == os.Stderr)
	return vbool(isInteractive), nil
}

// builtinReadDelimitedList implements read-delimited-list.
// (read-delimited-list char &optional input-stream recursive-p) => list
// Reads successive Lisp forms from input-stream until the delimiter
// character is encountered, returning a list of those forms.
// The delimiter is consumed from the stream.
func builtinReadDelimitedList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-delimited-list: need a delimiter character")
	}

	// Extract delimiter character
	delimitChar := primaryValue(args[0])
	var delim rune
	if delimitChar.typ == VChar {
		delim = delimitChar.ch
	} else if delimitChar.typ == VStr && len(delimitChar.str) == 1 {
		delim = rune(delimitChar.str[0])
	} else {
		return nil, fmt.Errorf("read-delimited-list: first argument must be a character")
	}

	// Validate: delimiter must be a terminating macro character
	rt := currentReadtable
	if rt == nil {
		return nil, fmt.Errorf("read-delimited-list: no readtable")
	}
	entry, ok := rt.macroFns[delim]
	if !ok || entry == nil {
		return nil, fmt.Errorf("read-delimited-list: %q is not a macro character", string(delim))
	}
	if !entry.terminating {
		return nil, fmt.Errorf("read-delimited-list: %q is not a terminating macro character", string(delim))
	}

	// Determine input stream (default: *standard-input*)
	stream := stdinStream
	if len(args) > 1 {
		stream = primaryValue(args[1])
		if stream.typ != VStream {
			return nil, fmt.Errorf("read-delimited-list: second argument must be a stream")
		}
	}

	// Read all remaining characters from the stream into a buffer
	var buf []rune
	for {
		ch, err := stream.stream.readChar()
		if err != nil {
			break
		}
		buf = append(buf, ch)
	}

	// Parse forms from the buffer using our delimited-list parser
	forms, err := readFormsFromRDP(buf, delim, rt)
	if err != nil {
		return nil, err
	}

	// Build a proper Lisp list from the forms
	var head, tail *Value
	for _, form := range forms {
		pair := cons(form, vnil())
		if head == nil {
			head = pair
			tail = pair
		} else {
			tail.cdr = pair
			tail = pair
		}
	}
	if head == nil {
		return vnil(), nil
	}
	return head, nil
}

// delimitedParser is a parser for read-delimited-list that reads from a rune slice.
type delimitedParser struct {
	src       []rune
	pos       int
	depth     int
	readtable *Readtable
}

// readFormsFromRDP reads all complete forms from the rune slice, stopping
// when the delimiter is encountered at depth 0.
func readFormsFromRDP(src []rune, delim rune, rt *Readtable) ([]*Value, error) {
	p := &delimitedParser{src: src, pos: 0, depth: 0, readtable: rt}
	forms := []*Value{}

	for p.pos < len(p.src) {
		// Skip whitespace
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
			p.pos++
		}
		if p.pos >= len(p.src) {
			break
		}

		// Check delimiter
		if p.src[p.pos] == delim && p.depth == 0 {
			p.pos++ // consume delimiter
			break
		}

		// Read one form
		form, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		if form != nil {
			forms = append(forms, form)
		}
	}

	return forms, nil
}

// readExpr reads one expression from the delimited parser.
func (p *delimitedParser) readExpr() (*Value, error) {
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected end of input")
	}

	ch := p.src[p.pos]

	// Skip whitespace
	for (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') && p.pos < len(p.src) {
		p.pos++
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unexpected end of input")
		}
		ch = p.src[p.pos]
	}

	switch ch {
	case '(':
		return p.readList('(', ')')
	case '[':
		return p.readList('[', ']')
	case '{':
		return p.readList('{', '}')
	case '\'':
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("quote"), cons(v, vnil())), nil
	case '`':
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("QUASIQUOTE"), cons(v, vnil())), nil
	case ',':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '@' {
			p.pos += 2
			v, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			return cons(vsym("UNQUOTE-SPLICING"), cons(v, vnil())), nil
		}
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("UNQUOTE"), cons(v, vnil())), nil
	case '#':
		return p.readDispatch()
	case '"':
		return p.readString()
	case '|':
		return p.readBarSym()
	case ';':
		// Comment — skip to end of line
		p.pos++
		for p.pos < len(p.src) && p.src[p.pos] != '\n' {
			p.pos++
		}
		// After comment, try to read next expression
		return p.readExpr()
	}

	// Number or symbol
	start := p.pos
	if (ch == '-' || ch == '+') && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1]) {
		p.pos++
		for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
			p.pos++
		}
		numStr := string(p.src[start:p.pos])
		if v, err := parseExpr(numStr); err == nil {
			return v, nil
		}
	}
	if isDigit(ch) || (ch == '-' && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1])) {
		p.pos++
		for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
			p.pos++
		}
		// Check for rational
		if p.pos < len(p.src) && p.src[p.pos] == '/' {
			p.pos++
			for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
				p.pos++
			}
		}
		// Check for decimal
		if p.pos < len(p.src) && p.src[p.pos] == '.' {
			p.pos++
			for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
				p.pos++
			}
		}
		// Check for exponent
		if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E' || p.src[p.pos] == 'd' || p.src[p.pos] == 'D') {
			p.pos++
			if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
				p.pos++
			}
			for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
				p.pos++
			}
		}
		numStr := string(p.src[start:p.pos])
		if v, err := parseExpr(numStr); err == nil {
			return v, nil
		}
	}

	// Symbol
	if isSymbolChar(ch) {
		p.pos++
		for p.pos < len(p.src) && isSymbolChar(p.src[p.pos]) {
			p.pos++
		}
		symStr := string(p.src[start:p.pos])
		return internOrVsym(symStr), nil
	}

	// Unknown char — skip and try next
	p.pos++
	return p.readExpr()
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isSymbolChar(ch rune) bool {
	if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '=' || ch == '<' || ch == '>' || ch == '_' || ch == '@' || ch == '#' || ch == '$' || ch == '%' || ch == '^' || ch == '&' || ch == '~' || ch == '.' || ch == '?' || ch == '!' || ch == ':'
}

func (p *delimitedParser) readList(open, close rune) (*Value, error) {
	p.pos++ // skip open
	p.depth++
	var head, tail *Value
	for p.pos < len(p.src) {
		// Skip whitespace
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
			p.pos++
		}
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] == close {
			p.pos++ // skip close
			p.depth--
			if head == nil {
				return vnil(), nil
			}
			return head, nil
		}
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		if v != nil {
			pair := cons(v, vnil())
			if head == nil {
				head = pair
				tail = pair
			} else {
				tail.cdr = pair
				tail = pair
			}
		}
	}
	return nil, fmt.Errorf("unclosed list")
}

func (p *delimitedParser) readString() (*Value, error) {
	p.pos++ // skip opening "
	var b strings.Builder
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			p.pos++ // skip backslash
			esc := p.src[p.pos]
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteRune(esc)
			}
			p.pos++ // skip escaped char
		} else if ch == '"' {
			p.pos++ // skip closing "
			return vstr(b.String()), nil
		} else {
			b.WriteRune(ch)
			p.pos++
		}
	}
	return nil, fmt.Errorf("unclosed string")
}

func (p *delimitedParser) readBarSym() (*Value, error) {
	p.pos++ // skip opening |
	var b strings.Builder
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			// Escape sequence: skip backslash and next char
			p.pos += 2
			b.WriteRune(p.src[p.pos-1])
		} else if ch == '|' {
			p.pos++ // skip closing |
			return vsym(b.String()), nil
		} else {
			b.WriteRune(ch)
			p.pos++
		}
	}
	return nil, fmt.Errorf("unclosed bar symbol")
}

func (p *delimitedParser) readDispatch() (*Value, error) {
	if p.pos+1 >= len(p.src) {
		p.pos++
		return internOrVsym("#"), nil
	}
	p.pos++ // skip #
	ch := p.src[p.pos]

	if ch == '\\' {
		// Character literal #\x or #\Space
		p.pos++
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != ' ' && p.src[p.pos] != '\t' && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' && p.src[p.pos] != '(' && p.src[p.pos] != ')' && p.src[p.pos] != '"' && p.src[p.pos] != '[' && p.src[p.pos] != ']' {
			p.pos++
		}
		name := string(p.src[start:p.pos])
		// Named character lookup (case-insensitive)
		switch strings.ToLower(name) {
		case "space":
			return vchar(' '), nil
		case "newline":
			return vchar('\n'), nil
		case "tab":
			return vchar('\t'), nil
		case "return":
			return vchar('\r'), nil
		case "backspace":
			return vchar('\x08'), nil
		case "rubout", "del":
			return vchar('\x7f'), nil
		}
		if len(name) == 1 {
			return vchar(rune(name[0])), nil
		}
		// Unknown multi-character name — treat as symbol
		return vsym("#" + "\\" + name), nil
	}

	if ch == '(' {
		// Vector literal #(...)
		p.pos++
		var elements []*Value
		for p.pos < len(p.src) {
			for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
				p.pos++
			}
			if p.pos >= len(p.src) {
				break
			}
			if p.src[p.pos] == ')' {
				p.pos++
				break
			}
			v, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			if v != nil {
				elements = append(elements, v)
			}
		}
		arr := &LispArray{dims: []int{len(elements)}, elements: make([]*Value, len(elements)), fillPtr: -1}
		copy(arr.elements, elements)
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	}

	if ch == '|' {
		// Block comment #| ... |#
		p.pos++
		depth := 1
		for p.pos+1 < len(p.src) && depth > 0 {
			if p.src[p.pos] == '|' && p.src[p.pos+1] == '#' {
				p.pos += 2
				depth--
			} else if p.src[p.pos] == '#' && p.src[p.pos+1] == '|' {
				p.pos += 2
				depth++
			} else {
				p.pos++
			}
		}
		// After block comment, read next expression
		return p.readExpr()
	}

	if ch == '+' || ch == '-' {
		// Feature conditional #+ or #-
		inc := (ch == '+')
		p.pos++
		// Read feature name
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != ' ' && p.src[p.pos] != '\t' && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' && p.src[p.pos] != '(' && p.src[p.pos] != ')' {
			p.pos++
		}
		featName := string(p.src[start:p.pos])
		// Read the conditional form
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
			p.pos++
		}
		form, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		satisfied := featureSatisfied(internOrVsym(":" + featName))
		if inc == satisfied {
			return form, nil
		}
		return p.readExpr()
	}

	if ch == 'C' || ch == 'c' {
		// Complex number #C(real imag)
		p.pos++
		if p.pos < len(p.src) && p.src[p.pos] == '(' {
			p.pos++
			realPart, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			imagPart, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			// Expect ')'
			for p.pos < len(p.src) && p.src[p.pos] != ')' {
				p.pos++
			}
			if p.pos < len(p.src) {
				p.pos++
			}
			r := 0.0
			if realPart != nil && realPart.typ == VNum {
				r = realPart.num
			}
			i := 0.0
			if imagPart != nil && imagPart.typ == VNum {
				i = imagPart.num
			}
			return vcomplex(r, i), nil
		}
	}

	if ch == 'P' || ch == 'p' {
		// Pathname #P"..."
		p.pos++
		if p.pos < len(p.src) && p.src[p.pos] == '"' {
			p.pos++
			var b strings.Builder
			for p.pos < len(p.src) && p.src[p.pos] != '"' {
				if p.src[p.pos] == '\\' && p.pos+1 < len(p.src) {
					p.pos++
				}
				b.WriteRune(p.src[p.pos])
				p.pos++
			}
			if p.pos < len(p.src) {
				p.pos++
			}
			return vpathname(parsePathnameString(b.String())), nil
		}
	}

	if ch == '\'' {
		// Function shorthand #'expr
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("function"), cons(v, vnil())), nil
	}

	if ch == '.' {
		// Sharp-dot #.expr — read and evaluate expr immediately
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		result, err := Eval(v, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("#. read-time evaluation error: %v", err)
		}
		return result, nil
	}

	p.pos++
	return internOrVsym("#" + string(ch)), nil
}

// buildListFromForms converts a slice of forms to a proper Lisp list.
func buildListFromForms(forms []*Value) *Value {
	var head, tail *Value
	for _, form := range forms {
		pair := cons(form, vnil())
		if head == nil {
			head = pair
			tail = pair
		} else {
			tail.cdr = pair
			tail = pair
		}
	}
	if head == nil {
		return vnil()
	}
	return head
}
