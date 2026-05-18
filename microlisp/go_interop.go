package microlisp

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"hash/crc64"
	"io"
	"io/fs"
	"log"
	"math"
	"math/big"
	"math/bits"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"regexp/syntax"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"text/scanner"
	"text/tabwriter"
	"text/template"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// GoPackageRegistry maps package name -> symbol name -> reflect.Value.
// Mirrors golisp's gopkg.Packages design for complete stdlib coverage.
var GoPackageRegistry = make(map[string]map[string]reflect.Value)

// GoTypeRegistry maps package name -> type name -> reflect.Type.
// Used for go:new and type introspection.
var GoTypeRegistry = make(map[string]map[string]reflect.Type)

func init() {
	registerFmt()
	registerStrings()
	registerStrconv()
	registerMath()
	registerMathBig()
	registerMathRand()
	registerMathBits()
	registerOS()
	registerIO()
	registerBytes()
	registerRegexp()
	registerSort()
	registerTime()
	registerNet()
	registerNetHTTP()
	registerNetURL()
	registerEncodingJSON()
	registerEncodingXML()
	registerEncodingCSV()
	registerEncodingBase64()
	registerLog()
	registerSync()
	registerRuntime()
	registerErrors()
	registerFlag()
	registerPath()
	registerFilepath()
	registerContext()
	registerOSExec()
	registerOSSignal()
	registerUnicode()
	registerUTF8()
	registerUTF16()
	registerFS()
	registerHash()
	registerCryptoHash()
	registerTextTemplate()
	registerTextScanner()
	registerTextTabwriter()
	registerSyscall()
	registerTesting()
	registerDebug()
	registerSlices()
}

// registerPackage adds all entries to GoPackageRegistry[packageName].
func registerPackage(name string, entries map[string]interface{}) {
	pkg, ok := GoPackageRegistry[name]
	if !ok {
		pkg = make(map[string]reflect.Value)
		GoPackageRegistry[name] = pkg
	}
	for sym, fn := range entries {
		pkg[sym] = reflect.ValueOf(fn)
	}
}

// -------- fmt --------
func registerFmt() {
	registerPackage("fmt", map[string]interface{}{
		"Sprintf":    fmt.Sprintf,
		"Printf":     fmt.Printf,
		"Fprintf":    fmt.Fprintf,
		"Sprint":     fmt.Sprint,
		"Print":      fmt.Print,
		"Fprint":     fmt.Fprint,
		"Println":    fmt.Println,
		"Fprintln":   fmt.Fprintln,
		"Sprintln":   fmt.Sprintln,
		"Errorf":     fmt.Errorf,
		"Fscan":      fmt.Fscan,
		"Fscanf":     fmt.Fscanf,
		"Fscanln":    fmt.Fscanln,
		"Scan":       fmt.Scan,
		"Scanf":      fmt.Scanf,
		"Scanln":     fmt.Scanln,
		"Sscan":      fmt.Sscan,
		"Sscanf":     fmt.Sscanf,
		"Sscanln":    fmt.Sscanln,
	})
}

// -------- strings --------
func registerStrings() {
	registerPackage("strings", map[string]interface{}{
		"Contains":       strings.Contains,
		"ContainsAny":    strings.ContainsAny,
		"ContainsRune":   strings.ContainsRune,
		"Count":          strings.Count,
		"EqualFold":      strings.EqualFold,
		"Fields":         strings.Fields,
		"FieldsFunc":     strings.FieldsFunc,
		"HasPrefix":      strings.HasPrefix,
		"HasSuffix":      strings.HasSuffix,
		"Index":          strings.Index,
		"IndexAny":       strings.IndexAny,
		"IndexByte":      strings.IndexByte,
		"IndexFunc":      strings.IndexFunc,
		"IndexRune":      strings.IndexRune,
		"Join":           strings.Join,
		"LastIndex":      strings.LastIndex,
		"LastIndexAny":   strings.LastIndexAny,
		"LastIndexFunc":  strings.LastIndexFunc,
		"Map":            strings.Map,
		"NewReader":      strings.NewReader,
		"NewReplacer":    strings.NewReplacer,
		"Repeat":         strings.Repeat,
		"Replace":        strings.Replace,
		"ReplaceAll":     strings.ReplaceAll,
		"Split":          strings.Split,
		"SplitAfter":     strings.SplitAfter,
		"SplitAfterN":    strings.SplitAfterN,
		"SplitN":         strings.SplitN,
		"Title":          strings.Title,
		"ToTitle":        strings.ToTitle,
		"ToLower":        strings.ToLower,
		"ToLowerSpecial": strings.ToLowerSpecial,
		"ToTitleSpecial": strings.ToTitleSpecial,
		"ToUpper":        strings.ToUpper,
		"ToUpperSpecial": strings.ToUpperSpecial,
		"Trim":           strings.Trim,
		"TrimFunc":       strings.TrimFunc,
		"TrimLeft":       strings.TrimLeft,
		"TrimLeftFunc":   strings.TrimLeftFunc,
		"TrimPrefix":     strings.TrimPrefix,
		"TrimRight":      strings.TrimRight,
		"TrimRightFunc":  strings.TrimRightFunc,
		"TrimSpace":      strings.TrimSpace,
		"TrimSuffix":     strings.TrimSuffix,
		"Cut":            strings.Cut,
		"CutPrefix":      strings.CutPrefix,
		"CutSuffix":      strings.CutSuffix,
		"Clone":          strings.Clone,
		"ToValidUTF8":    strings.ToValidUTF8,
	})
}

// -------- strconv --------
func registerStrconv() {
	registerPackage("strconv", map[string]interface{}{
		"Atoi":           strconv.Atoi,
		"Itoa":           strconv.Itoa,
		"ParseBool":      strconv.ParseBool,
		"ParseFloat":     strconv.ParseFloat,
		"ParseInt":       strconv.ParseInt,
		"ParseUint":      strconv.ParseUint,
		"FormatBool":     strconv.FormatBool,
		"FormatFloat":    strconv.FormatFloat,
		"FormatInt":      strconv.FormatInt,
		"FormatUint":     strconv.FormatUint,
		"Quote":          strconv.Quote,
		"QuoteToASCII":   strconv.QuoteToASCII,
		"QuoteToGraphic": strconv.QuoteToGraphic,
		"Unquote":        strconv.Unquote,
		"AppendBool":     strconv.AppendBool,
		"AppendFloat":    strconv.AppendFloat,
		"AppendInt":      strconv.AppendInt,
		"AppendUint":     strconv.AppendUint,
		"AppendQuote":    strconv.AppendQuote,
		"IsPrint":        strconv.IsPrint,
		"CanBackquote":   strconv.CanBackquote,
	})
}

// -------- math --------
func registerMath() {
	registerPackage("math", map[string]interface{}{
		"Sin":       math.Sin,
		"Cos":       math.Cos,
		"Tan":       math.Tan,
		"Asin":      math.Asin,
		"Acos":      math.Acos,
		"Atan":      math.Atan,
		"Atan2":     math.Atan2,
		"Sinh":      math.Sinh,
		"Cosh":      math.Cosh,
		"Tanh":      math.Tanh,
		"Asinh":     math.Asinh,
		"Acosh":     math.Acosh,
		"Atanh":     math.Atanh,
		"Sqrt":      math.Sqrt,
		"Cbrt":      math.Cbrt,
		"Hypot":     math.Hypot,
		"Exp":       math.Exp,
		"Exp2":      math.Exp2,
		"Expm1":     math.Expm1,
		"Log":       math.Log,
		"Log10":     math.Log10,
		"Log2":      math.Log2,
		"Log1p":     math.Log1p,
		"Logb":      math.Logb,
		"Pow":       math.Pow,
		"Pow10":     math.Pow10,
		"Abs":       math.Abs,
		"Ceil":      math.Ceil,
		"Floor":     math.Floor,
		"Trunc":     math.Trunc,
		"Round":     math.Round,
		"RoundToEven": math.RoundToEven,
		"Mod":       math.Mod,
		"Remainder": math.Remainder,
		"Max":       math.Max,
		"Min":       math.Min,
		"Dim":       math.Dim,
		"Frexp":     math.Frexp,
		"Ldexp":     math.Ldexp,
		"Modf":      math.Modf,
		"Gamma":     math.Gamma,
		"Lgamma":    math.Lgamma,
		"Erf":       math.Erf,
		"Erfc":      math.Erfc,
		"J0":        math.J0,
		"J1":        math.J1,
		"Y0":        math.Y0,
		"Y1":        math.Y1,
		"Nextafter": math.Nextafter,
		"Nextafter32": math.Nextafter32,
		"Signbit":   math.Signbit,
		"Copysign":  math.Copysign,
		"Inf":       math.Inf,
		"IsInf":     math.IsInf,
		"IsNaN":     math.IsNaN,
		"NaN":       math.NaN,
		"Float64bits":  math.Float64bits,
		"Float64frombits": math.Float64frombits,
		"Float32bits":  math.Float32bits,
		"Float32frombits": math.Float32frombits,
		"Ilcm":      math.Ilogb,
		"Sqrt2":     math.Sqrt2,
		"E":         math.E,
		"Pi":        math.Pi,
		"Ln2":       math.Ln2,
		"Log2E":     math.Log2E,
		"Log10E":    math.Log10E,
		"Ln10":      math.Ln10,
		"MaxInt":    math.MaxInt,
		"MinInt":    math.MinInt,
		"MaxFloat64": math.MaxFloat64,
		"SmallestNonzeroFloat64": math.SmallestNonzeroFloat64,
		"MaxFloat32": math.MaxFloat32,
		"SmallestNonzeroFloat32": math.SmallestNonzeroFloat32,
	})
}

// -------- math/big --------
func registerMathBig() {
	registerPackage("math/big", map[string]interface{}{
		"NewInt":  big.NewInt,
		"NewFloat": big.NewFloat,
		"NewRat":  big.NewRat,
	})
	registerType("math/big", "Int", reflect.TypeOf(&big.Int{}))
	registerType("math/big", "Float", reflect.TypeOf(&big.Float{}))
	registerType("math/big", "Rat", reflect.TypeOf(&big.Rat{}))
}

// -------- math/rand --------
func registerMathRand() {
	registerPackage("math/rand", map[string]interface{}{
		"Int":       rand.Int,
		"Int63":     rand.Int63,
		"Int63n":    rand.Int63n,
		"Int31":     rand.Int31,
		"Int31n":    rand.Int31n,
		"Intn":      rand.Intn,
		"Float64":   rand.Float64,
		"Float32":   rand.Float32,
		"Uint32":    rand.Uint32,
		"Uint64":    rand.Uint64,
		"ExpFloat64": rand.ExpFloat64,
		"NormFloat64": rand.NormFloat64,
		"Perm":      rand.Perm,
		"Shuffle":   rand.Shuffle,
		"Read":      rand.Read,
		"Seed":      rand.Seed,
		"Rand":      rand.New,
	})
	registerType("math/rand", "Rand", reflect.TypeOf(&rand.Rand{}))
}

// -------- math/bits --------
func registerMathBits() {
	registerPackage("math/bits", map[string]interface{}{
		"UintSize":       bits.UintSize,
		"Len":            bits.Len,
		"Len8":           bits.Len8,
		"Len16":          bits.Len16,
		"Len32":          bits.Len32,
		"Len64":          bits.Len64,
		"OnesCount":      bits.OnesCount,
		"OnesCount8":     bits.OnesCount8,
		"OnesCount16":    bits.OnesCount16,
		"OnesCount32":    bits.OnesCount32,
		"OnesCount64":    bits.OnesCount64,
		"TrailingZeros":  bits.TrailingZeros,
		"TrailingZeros8": bits.TrailingZeros8,
		"TrailingZeros16": bits.TrailingZeros16,
		"TrailingZeros32": bits.TrailingZeros32,
		"TrailingZeros64": bits.TrailingZeros64,
		"LeadingZeros":   bits.LeadingZeros,
		"LeadingZeros8":  bits.LeadingZeros8,
		"LeadingZeros16": bits.LeadingZeros16,
		"LeadingZeros32": bits.LeadingZeros32,
		"LeadingZeros64": bits.LeadingZeros64,
		"Reverse":        bits.Reverse,
		"Reverse8":       bits.Reverse8,
		"Reverse16":      bits.Reverse16,
		"Reverse32":      bits.Reverse32,
		"Reverse64":      bits.Reverse64,
		"ReverseBytes":   bits.ReverseBytes,
		"ReverseBytes16": bits.ReverseBytes16,
		"ReverseBytes32": bits.ReverseBytes32,
		"ReverseBytes64": bits.ReverseBytes64,
		"RotateLeft":     bits.RotateLeft,
		"RotateLeft8":    bits.RotateLeft8,
		"RotateLeft16":   bits.RotateLeft16,
		"RotateLeft32":   bits.RotateLeft32,
		"RotateLeft64":   bits.RotateLeft64,
		"Add":            bits.Add,
		"Add64":          bits.Add64,
		"Sub":            bits.Sub,
		"Sub64":          bits.Sub64,
		"Mul":            bits.Mul,
		"Mul64":          bits.Mul64,
		"Div":            bits.Div,
		"Div64":          bits.Div64,
		"Rem":            bits.Rem,
		"Rem64":          bits.Rem64,
	})
}

// -------- os --------
func registerOS() {
	registerPackage("os", map[string]interface{}{
		"Args":            os.Args,
		"Chdir":           os.Chdir,
		"Chmod":           os.Chmod,
		"Chown":           os.Chown,
		"Chtimes":         os.Chtimes,
		"Clearenv":        os.Clearenv,
		"Create":          os.Create,
		"DevNull":         os.DevNull,
		"Environ":         os.Environ,
		"ErrExist":        os.ErrExist,
		"ErrInvalid":      os.ErrInvalid,
		"ErrNotExist":     os.ErrNotExist,
		"ErrPermission":   os.ErrPermission,
		"Exit":            os.Exit,
		"Expand":          os.Expand,
		"ExpandEnv":       os.ExpandEnv,
		"FindProcess":     os.FindProcess,
		"Getegid":         os.Getegid,
		"Getenv":          os.Getenv,
		"Geteuid":         os.Geteuid,
		"Getgid":          os.Getgid,
		"Getgroups":       os.Getgroups,
		"Getpagesize":     os.Getpagesize,
		"Getpid":          os.Getpid,
		"Getuid":          os.Getuid,
		"Getwd":           os.Getwd,
		"Hostname":        os.Hostname,
		"Interrupt":       os.Interrupt,
		"IsExist":         os.IsExist,
		"IsNotExist":      os.IsNotExist,
		"IsPathSeparator": os.IsPathSeparator,
		"IsPermission":    os.IsPermission,
		"Kill":            os.Kill,
		"Lchown":          os.Lchown,
		"Link":            os.Link,
		"Lstat":           os.Lstat,
		"Mkdir":           os.Mkdir,
		"MkdirAll":        os.MkdirAll,
		"NewFile":         os.NewFile,
		"NewSyscallError": os.NewSyscallError,
		"Open":            os.Open,
		"OpenFile":        os.OpenFile,
		"PathListSeparator": os.PathListSeparator,
		"PathSeparator":   os.PathSeparator,
		"Pipe":            os.Pipe,
		"Readlink":        os.Readlink,
		"Remove":          os.Remove,
		"RemoveAll":       os.RemoveAll,
		"Rename":          os.Rename,
		"SameFile":        os.SameFile,
		"Setenv":          os.Setenv,
		"StartProcess":    os.StartProcess,
		"Stat":            os.Stat,
		"Stderr":          os.Stderr,
		"Stdin":           os.Stdin,
		"Stdout":          os.Stdout,
		"Symlink":         os.Symlink,
		"TempDir":         os.TempDir,
		"Truncate":        os.Truncate,
		"Unsetenv":        os.Unsetenv,
		"LinkError":       &os.LinkError{},
		"PathError":       &os.PathError{},
		"SyscallError":    &os.SyscallError{},
		// Mode constants
		"ModeAppend":      os.ModeAppend,
		"ModeCharDevice":  os.ModeCharDevice,
		"ModeDevice":      os.ModeDevice,
		"ModeDir":         os.ModeDir,
		"ModeExclusive":   os.ModeExclusive,
		"ModeNamedPipe":   os.ModeNamedPipe,
		"ModePerm":        os.ModePerm,
		"ModeSetgid":      os.ModeSetgid,
		"ModeSetuid":      os.ModeSetuid,
		"ModeSocket":      os.ModeSocket,
		"ModeSticky":      os.ModeSticky,
		"ModeSymlink":     os.ModeSymlink,
		"ModeTemporary":   os.ModeTemporary,
		"ModeType":        os.ModeType,
		// O constants
		"O_RDONLY": os.O_RDONLY,
		"O_WRONLY": os.O_WRONLY,
		"O_RDWR":   os.O_RDWR,
		"O_APPEND": os.O_APPEND,
		"O_CREATE": os.O_CREATE,
		"O_EXCL":   os.O_EXCL,
		"O_SYNC":   os.O_SYNC,
		"O_TRUNC":  os.O_TRUNC,
		// Seek constants
		"SEEK_CUR": os.SEEK_CUR,
		"SEEK_END": os.SEEK_END,
		"SEEK_SET": os.SEEK_SET,
	})
	registerType("os", "File", reflect.TypeOf(&os.File{}))
	registerType("os", "Process", reflect.TypeOf(&os.Process{}))
	registerType("os", "ProcessState", reflect.TypeOf(&os.ProcessState{}))
	registerType("os", "FileInfo", reflect.TypeOf((*fs.FileInfo)(nil)).Elem())
	registerType("os", "PathError", reflect.TypeOf(&os.PathError{}))
	registerType("os", "LinkError", reflect.TypeOf(&os.LinkError{}))
}

// -------- io --------
func registerIO() {
	registerPackage("io", map[string]interface{}{
		"Copy":         io.Copy,
		"CopyN":        io.CopyN,
		"ReadFull":     io.ReadFull,
		"ReadAtLeast":  io.ReadAtLeast,
		"WriteString":  io.WriteString,
		"Pipe":         io.Pipe,
		"LimitReader":  io.LimitReader,
		"TeeReader":    io.TeeReader,
		"MultiReader":  io.MultiReader,
		"MultiWriter":  io.MultiWriter,
		"EOF":          io.EOF,
		"ErrClosedPipe": io.ErrClosedPipe,
		"ErrNoProgress": io.ErrNoProgress,
		"ErrShortBuffer": io.ErrShortBuffer,
		"ErrShortWrite": io.ErrShortWrite,
		"ErrUnexpectedEOF": io.ErrUnexpectedEOF,
	})
	registerType("io", "Reader", reflect.TypeOf((*io.Reader)(nil)).Elem())
	registerType("io", "Writer", reflect.TypeOf((*io.Writer)(nil)).Elem())
	registerType("io", "ReadCloser", reflect.TypeOf((*io.ReadCloser)(nil)).Elem())
	registerType("io", "WriteCloser", reflect.TypeOf((*io.WriteCloser)(nil)).Elem())
	registerType("io", "ReadWriter", reflect.TypeOf((*io.ReadWriter)(nil)).Elem())
}

// -------- bytes --------
func registerBytes() {
	registerPackage("bytes", map[string]interface{}{
		"Contains":       bytes.Contains,
		"ContainsAny":    bytes.ContainsAny,
		"ContainsRune":   bytes.ContainsRune,
		"Count":          bytes.Count,
		"Equal":          bytes.Equal,
		"EqualFold":      bytes.EqualFold,
		"Fields":         bytes.Fields,
		"FieldsFunc":     bytes.FieldsFunc,
		"HasPrefix":      bytes.HasPrefix,
		"HasSuffix":      bytes.HasSuffix,
		"Index":          bytes.Index,
		"IndexAny":       bytes.IndexAny,
		"IndexByte":      bytes.IndexByte,
		"IndexFunc":      bytes.IndexFunc,
		"IndexRune":      bytes.IndexRune,
		"Join":           bytes.Join,
		"LastIndex":      bytes.LastIndex,
		"LastIndexAny":   bytes.LastIndexAny,
		"LastIndexFunc":  bytes.LastIndexFunc,
		"Map":            bytes.Map,
		"NewBuffer":      bytes.NewBuffer,
		"NewBufferString": bytes.NewBufferString,
		"NewReader":      bytes.NewReader,
		"Repeat":         bytes.Repeat,
		"Replace":        bytes.Replace,
		"ReplaceAll":     bytes.ReplaceAll,
		"Runes":          bytes.Runes,
		"Split":          bytes.Split,
		"SplitAfter":     bytes.SplitAfter,
		"SplitAfterN":    bytes.SplitAfterN,
		"SplitN":         bytes.SplitN,
		"Title":          bytes.Title,
		"ToLower":        bytes.ToLower,
		"ToTitle":        bytes.ToTitle,
		"ToUpper":        bytes.ToUpper,
		"Trim":           bytes.Trim,
		"TrimFunc":       bytes.TrimFunc,
		"TrimLeft":       bytes.TrimLeft,
		"TrimLeftFunc":   bytes.TrimLeftFunc,
		"TrimPrefix":     bytes.TrimPrefix,
		"TrimRight":      bytes.TrimRight,
		"TrimRightFunc":  bytes.TrimRightFunc,
		"TrimSpace":      bytes.TrimSpace,
		"TrimSuffix":     bytes.TrimSuffix,
		"Cut":            bytes.Cut,
		"CutPrefix":      bytes.CutPrefix,
		"CutSuffix":      bytes.CutSuffix,
		"Clone":          bytes.Clone,
		"ToValidUTF8":    bytes.ToValidUTF8,
		"Compare":        bytes.Compare,
	})
	registerType("bytes", "Buffer", reflect.TypeOf(&bytes.Buffer{}))
	registerType("bytes", "Reader", reflect.TypeOf(&bytes.Reader{}))
}

// -------- regexp --------
func registerRegexp() {
	registerPackage("regexp", map[string]interface{}{
		"Compile":        regexp.Compile,
		"CompilePOSIX":   regexp.CompilePOSIX,
		"MustCompile":    regexp.MustCompile,
		"MustCompilePOSIX": regexp.MustCompilePOSIX,
		"QuoteMeta":      regexp.QuoteMeta,
	})
	registerType("regexp", "Regexp", reflect.TypeOf(&regexp.Regexp{}))
	registerType("regexp/syntax", "Op", reflect.TypeOf(syntax.Op(0)))
}

// -------- sort --------
func registerSort() {
	registerPackage("sort", map[string]interface{}{
		"Ints":     sort.Ints,
		"Float64s": sort.Float64s,
		"Strings":  sort.Strings,
		"Search":   sort.Search,
		"SearchInts": sort.SearchInts,
		"SearchFloat64s": sort.SearchFloat64s,
		"SearchStrings":  sort.SearchStrings,
		"IntsAreSorted":  sort.IntsAreSorted,
		"Float64sAreSorted": sort.Float64sAreSorted,
		"StringsAreSorted":  sort.StringsAreSorted,
		"Slice":      sort.Slice,
		"SliceIsSorted": sort.SliceIsSorted,
		"SliceStable":   sort.SliceStable,
	})
	registerType("sort", "Interface", reflect.TypeOf((*sort.Interface)(nil)).Elem())
}

// -------- time --------
func registerTime() {
	registerPackage("time", map[string]interface{}{
		"Now":            time.Now,
		"Date":           time.Date,
		"Parse":          time.Parse,
		"ParseDuration":  time.ParseDuration,
		"Unix":           time.Unix,
		"UnixMilli":      time.UnixMilli,
		"UnixMicro":      time.UnixMicro,
		"Sleep":          time.Sleep,
		"After":          time.After,
		"Tick":           time.Tick,
		"Since":          time.Since,
		"Until":          time.Until,
		"FixedZone":      time.FixedZone,
		"LoadLocation":   time.LoadLocation,
		"NewTicker":      time.NewTicker,
		"NewTimer":       time.NewTimer,
		"AfterFunc":      time.AfterFunc,
		"Second":         time.Second,
		"Minute":         time.Minute,
		"Hour":           time.Hour,
		"Nanosecond":     time.Nanosecond,
		"Millisecond":    time.Millisecond,
		"Microsecond":    time.Microsecond,
		"ANSIC":          time.ANSIC,
		"UnixDate":       time.UnixDate,
		"RubyDate":       time.RubyDate,
		"RFC822":         time.RFC822,
		"RFC822Z":        time.RFC822Z,
		"RFC850":         time.RFC850,
		"RFC1123":        time.RFC1123,
		"RFC1123Z":       time.RFC1123Z,
		"RFC3339":        time.RFC3339,
		"RFC3339Nano":    time.RFC3339Nano,
		"Kitchen":        time.Kitchen,
		"Stamp":          time.Stamp,
		"StampMilli":     time.StampMilli,
		"StampMicro":     time.StampMicro,
		"StampNano":      time.StampNano,
		"Saturday":       time.Saturday,
		"Sunday":         time.Sunday,
		"Monday":         time.Monday,
		"January":        time.January,
		"February":       time.February,
		"UTC":            time.UTC,
		"Local":          time.Local,
	})
	registerType("time", "Time", reflect.TypeOf(time.Time{}))
	registerType("time", "Duration", reflect.TypeOf(time.Duration(0)))
	registerType("time", "Ticker", reflect.TypeOf(&time.Ticker{}))
	registerType("time", "Timer", reflect.TypeOf(&time.Timer{}))
	registerType("time", "Location", reflect.TypeOf(&time.Location{}))
}

// -------- net --------
func registerNet() {
	registerPackage("net", map[string]interface{}{
		"Dial":         net.Dial,
		"DialTCP":      net.DialTCP,
		"DialUDP":      net.DialUDP,
		"DialIP":       net.DialIP,
		"Listen":       net.Listen,
		"ListenTCP":    net.ListenTCP,
		"ListenUDP":    net.ListenUDP,
		"ListenIP":     net.ListenIP,
		"LookupHost":   net.LookupHost,
		"LookupIP":     net.LookupIP,
		"ResolveTCPAddr": net.ResolveTCPAddr,
		"ResolveUDPAddr": net.ResolveUDPAddr,
		"ResolveIPAddr":  net.ResolveIPAddr,
		"SplitHostPort": net.SplitHostPort,
		"JoinHostPort":  net.JoinHostPort,
		"ParseCIDR":     net.ParseCIDR,
		"ParseIP":       net.ParseIP,
		"InterfaceByName": net.InterfaceByName,
		"InterfaceAddrs": net.InterfaceAddrs,
	})
	registerType("net", "Listener", reflect.TypeOf((*net.Listener)(nil)).Elem())
	registerType("net", "Conn", reflect.TypeOf((*net.Conn)(nil)).Elem())
	registerType("net", "IP", reflect.TypeOf(net.IP{}))
	registerType("net", "IPAddr", reflect.TypeOf(&net.IPAddr{}))
	registerType("net", "TCPAddr", reflect.TypeOf(&net.TCPAddr{}))
	registerType("net", "UDPAddr", reflect.TypeOf(&net.UDPAddr{}))
}

// -------- net/http --------
func registerNetHTTP() {
	registerPackage("net/http", map[string]interface{}{
		"Get":              http.Get,
		"Head":             http.Head,
		"Post":             http.Post,
		"PostForm":         http.PostForm,
		"NewRequest":       http.NewRequest,
		"NewRequestWithContext": http.NewRequestWithContext,
		"DefaultClient":    http.DefaultClient,
		"DefaultServeMux":  http.DefaultServeMux,
		"ErrHandlerTimeout": http.ErrHandlerTimeout,
		"ErrMissingBoundary": http.ErrMissingBoundary,
		"ErrMissingContentLength": http.ErrMissingContentLength,
		"StatusText":       http.StatusText,
		"CanonicalHeaderKey": http.CanonicalHeaderKey,
		"SetCookie":        http.SetCookie,
		"DetectContentType": http.DetectContentType,
		"ProxyFromEnvironment": http.ProxyFromEnvironment,
		"ProxyURL":         http.ProxyURL,
		"Handle":           http.Handle,
		"HandleFunc":       http.HandleFunc,
		"ListenAndServe":   http.ListenAndServe,
		"ListenAndServeTLS": http.ListenAndServeTLS,
		"Serve":            http.Serve,
		"ServeTLS":         http.ServeTLS,
		"TimeoutHandler":   http.TimeoutHandler,
		"StripPrefix":      http.StripPrefix,
		"Redirect":         http.Redirect,
		"NotFound":         http.NotFound,
		"Error":            http.Error,
		"ServeContent":     http.ServeContent,
		"ServeFile":        http.ServeFile,
		"FileServer":       http.FileServer,
		"NewServeMux":      http.NewServeMux,
		"StatusNotFound":   http.StatusNotFound,
		"StatusBadRequest": http.StatusBadRequest,
		"StatusInternalServerError": http.StatusInternalServerError,
		"MethodGet":        http.MethodGet,
		"MethodPost":       http.MethodPost,
		"MethodPut":        http.MethodPut,
		"MethodDelete":     http.MethodDelete,
		"MethodHead":       http.MethodHead,
		"MethodPatch":      http.MethodPatch,
		"MethodOptions":    http.MethodOptions,
	})
	registerType("net/http", "Client", reflect.TypeOf(&http.Client{}))
	registerType("net/http", "Request", reflect.TypeOf(&http.Request{}))
	registerType("net/http", "Response", reflect.TypeOf(&http.Response{}))
	registerType("net/http", "Header", reflect.TypeOf(http.Header{}))
	registerType("net/http", "Transport", reflect.TypeOf(&http.Transport{}))
	registerType("net/http", "Server", reflect.TypeOf(&http.Server{}))
	registerType("net/http", "Cookie", reflect.TypeOf(&http.Cookie{}))
}

// -------- net/url --------
func registerNetURL() {
	registerPackage("net/url", map[string]interface{}{
		"Parse":          url.Parse,
		"ParseRequestURI": url.ParseRequestURI,
		"QueryEscape":    url.QueryEscape,
		"QueryUnescape":  url.QueryUnescape,
		"PathEscape":     url.PathEscape,
		"PathUnescape":   url.PathUnescape,
		"JoinPath":       url.JoinPath,
	})
	registerType("net/url", "URL", reflect.TypeOf(&url.URL{}))
	registerType("net/url", "Values", reflect.TypeOf(url.Values{}))
}

// -------- encoding/json --------
func registerEncodingJSON() {
	registerPackage("encoding/json", map[string]interface{}{
		"Marshal":          json.Marshal,
		"Unmarshal":        json.Unmarshal,
		"MarshalIndent":    json.MarshalIndent,
		"Valid":            json.Valid,
		"Indent":           json.Indent,
		"HTMLEscape":       json.HTMLEscape,
		"Compact":          json.Compact,
		"Number":           json.Number(""),
	})
	registerType("encoding/json", "RawMessage", reflect.TypeOf(json.RawMessage{}))
	registerType("encoding/json", "Decoder", reflect.TypeOf(&json.Decoder{}))
	registerType("encoding/json", "Encoder", reflect.TypeOf(&json.Encoder{}))
}

// -------- encoding/xml --------
func registerEncodingXML() {
	registerPackage("encoding/xml", map[string]interface{}{
		"Marshal":          xml.Marshal,
		"Unmarshal":        xml.Unmarshal,
		"MarshalIndent":    xml.MarshalIndent,
		"EscapeText":       xml.EscapeText,
	})
	registerType("encoding/xml", "Name", reflect.TypeOf(xml.Name{}))
}

// -------- encoding/csv --------
func registerEncodingCSV() {
	registerPackage("encoding/csv", map[string]interface{}{
		"NewReader": csv.NewReader,
		"NewWriter": csv.NewWriter,
		"ErrFieldCount": csv.ErrFieldCount,
		"ErrBareQuote":  csv.ErrBareQuote,
		"ErrQuote":      csv.ErrQuote,
	})
}

// -------- encoding/base64 --------
func registerEncodingBase64() {
	registerPackage("encoding/base64", map[string]interface{}{
		"StdEncoding":  base64.StdEncoding,
		"URLEncoding":  base64.URLEncoding,
		"RawStdEncoding": base64.RawStdEncoding,
		"RawURLEncoding": base64.RawURLEncoding,
		"NewDecoder":   base64.NewDecoder,
		"NewEncoder":   base64.NewEncoder,
		"StdEncoding.EncodeToString": base64.StdEncoding.EncodeToString,
		"StdEncoding.DecodeString": base64.StdEncoding.DecodeString,
	})
}

// -------- log --------
func registerLog() {
	registerPackage("log", map[string]interface{}{
		"Print":        log.Print,
		"Printf":       log.Printf,
		"Println":      log.Println,
		"Fatal":        log.Fatal,
		"Fatalf":       log.Fatalf,
		"Fatalln":      log.Fatalln,
		"Panic":        log.Panic,
		"Panicf":       log.Panicf,
		"Panicln":      log.Panicln,
		"SetFlags":     log.SetFlags,
		"SetPrefix":    log.SetPrefix,
		"SetOutput":    log.SetOutput,
		"Flags":        log.Flags,
		"Prefix":       log.Prefix,
		"Output":       log.Output,
		"New":          log.New,
		"Ldate":        log.Ldate,
		"Ltime":        log.Ltime,
		"Lmicroseconds": log.Lmicroseconds,
		"Llongfile":    log.Llongfile,
		"Lshortfile":   log.Lshortfile,
		"LUTC":         log.LUTC,
		"Lmsgprefix":   log.Lmsgprefix,
		"LstdFlags":    log.LstdFlags,
	})
}

// -------- sync --------
func registerSync() {
	registerPackage("sync", map[string]interface{}{
		"NewCond":  sync.NewCond,
	})
	registerType("sync", "Mutex", reflect.TypeOf(&sync.Mutex{}))
	registerType("sync", "RWMutex", reflect.TypeOf(&sync.RWMutex{}))
	registerType("sync", "WaitGroup", reflect.TypeOf(&sync.WaitGroup{}))
	registerType("sync", "Once", reflect.TypeOf(&sync.Once{}))
	registerType("sync", "Cond", reflect.TypeOf(&sync.Cond{}))
	registerType("sync", "Map", reflect.TypeOf(&sync.Map{}))
	registerType("sync", "Pool", reflect.TypeOf(&sync.Pool{}))
}

// -------- runtime --------
func registerRuntime() {
	registerPackage("runtime", map[string]interface{}{
		"GOOS":         runtime.GOOS,
		"GOARCH":       runtime.GOARCH,
		"GOMAXPROCS":   runtime.GOMAXPROCS,
		"NumCPU":       runtime.NumCPU,
		"NumGoroutine": runtime.NumGoroutine,
		"NumCgoCall":   runtime.NumCgoCall,
		"Version":      runtime.Version,
		"Caller":       runtime.Caller,
		"FuncForPC":    runtime.FuncForPC,
		"GC":           runtime.GC,
		"LockOSThread": runtime.LockOSThread,
		"UnlockOSThread": runtime.UnlockOSThread,
		"Gosched":      runtime.Gosched,
		"Goexit":       runtime.Goexit,
		"MemStats":     &runtime.MemStats{},
		"ReadMemStats": runtime.ReadMemStats,
		"SetFinalizer": runtime.SetFinalizer,
		"KeepAlive":    runtime.KeepAlive,
	})
	registerType("runtime", "MemStats", reflect.TypeOf(&runtime.MemStats{}))
}

// -------- errors --------
func registerErrors() {
	registerPackage("errors", map[string]interface{}{
		"New":        errors.New,
		"Is":         errors.Is,
		"As":         errors.As,
		"Unwrap":     errors.Unwrap,
		"Join":       errors.Join,
	})
}

// -------- flag --------
func registerFlag() {
	registerPackage("flag", map[string]interface{}{
		"Parse":        flag.Parse,
		"Bool":         flag.Bool,
		"String":       flag.String,
		"Int":          flag.Int,
		"Int64":        flag.Int64,
		"Uint":         flag.Uint,
		"Float64":      flag.Float64,
		"Duration":     flag.Duration,
		"BoolVar":      flag.BoolVar,
		"StringVar":    flag.StringVar,
		"IntVar":       flag.IntVar,
		"Int64Var":     flag.Int64Var,
		"UintVar":      flag.UintVar,
		"Float64Var":   flag.Float64Var,
		"DurationVar":  flag.DurationVar,
		"PrintDefaults": flag.PrintDefaults,
		"NFlag":        flag.NFlag,
		"Arg":          flag.Arg,
		"Args":         flag.Args,
		"NArg":         flag.NArg,
		"CommandLine":  flag.CommandLine,
		"NewFlagSet":   flag.NewFlagSet,
		"VisitAll":     flag.VisitAll,
		"Visit":        flag.Visit,
		"Lookup":       flag.Lookup,
		"Set":          flag.Set,
	})
}

// -------- path --------
func registerPath() {
	registerPackage("path", map[string]interface{}{
		"Base":       path.Base,
		"Clean":      path.Clean,
		"Dir":        path.Dir,
		"Ext":        path.Ext,
		"IsAbs":      path.IsAbs,
		"Join":       path.Join,
		"Match":      path.Match,
		"Split":      path.Split,
	})
}

// -------- filepath --------
func registerFilepath() {
	registerPackage("path/filepath", map[string]interface{}{
		"Abs":             filepath.Abs,
		"Base":            filepath.Base,
		"Clean":           filepath.Clean,
		"Dir":             filepath.Dir,
		"EvalSymlinks":    filepath.EvalSymlinks,
		"Ext":             filepath.Ext,
		"FromSlash":       filepath.FromSlash,
		"Glob":            filepath.Glob,
		"HasPrefix":       filepath.HasPrefix,
		"IsAbs":           filepath.IsAbs,
		"Join":            filepath.Join,
		"Match":           filepath.Match,
		"Rel":             filepath.Rel,
		"Split":           filepath.Split,
		"SplitList":       filepath.SplitList,
		"ToSlash":         filepath.ToSlash,
		"VolumeName":      filepath.VolumeName,
		"Walk":            filepath.Walk,
		"WalkDir":         filepath.WalkDir,
		"ListSeparator":   filepath.ListSeparator,
		"Separator":       filepath.Separator,
	})
}

// -------- context --------
func registerContext() {
	registerPackage("context", map[string]interface{}{
		"Background":  context.Background,
		"TODO":        context.TODO,
		"WithCancel":  context.WithCancel,
		"WithTimeout": context.WithTimeout,
		"WithDeadline": context.WithDeadline,
		"WithValue":   context.WithValue,
		"Canceled":    context.Canceled,
		"DeadlineExceeded": context.DeadlineExceeded,
	})
	registerType("context", "Context", reflect.TypeOf((*context.Context)(nil)).Elem())
}

// -------- os/exec --------
func registerOSExec() {
	registerPackage("os/exec", map[string]interface{}{
		"Command":        exec.Command,
		"CommandContext": exec.CommandContext,
		"LookPath":       exec.LookPath,
	})
	registerType("os/exec", "Cmd", reflect.TypeOf(&exec.Cmd{}))
	registerType("os/exec", "ExitError", reflect.TypeOf(&exec.ExitError{}))
}

// -------- os/signal --------
func registerOSSignal() {
	registerPackage("os/signal", map[string]interface{}{
		"Notify":  signal.Notify,
		"Stop":    signal.Stop,
		"Reset":   signal.Reset,
	})
}

// -------- unicode --------
func registerUnicode() {
	registerPackage("unicode", map[string]interface{}{
		"IsDigit":        unicode.IsDigit,
		"IsLetter":       unicode.IsLetter,
		"IsLower":        unicode.IsLower,
		"IsUpper":        unicode.IsUpper,
		"IsSpace":        unicode.IsSpace,
		"IsPunct":        unicode.IsPunct,
		"IsSymbol":       unicode.IsSymbol,
		"IsMark":         unicode.IsMark,
		"IsGraphic":      unicode.IsGraphic,
		"IsControl":      unicode.IsControl,
		"IsPrint":        unicode.IsPrint,
		"IsOneOf":        unicode.IsOneOf,
		"To":             unicode.To,
		"ToLower":        unicode.ToLower,
		"ToTitle":        unicode.ToTitle,
		"ToUpper":        unicode.ToUpper,
		"Is":             unicode.Is,
		"SimpleFold":     unicode.SimpleFold,
		"IsNumber":       unicode.IsNumber,
	})
	registerType("unicode", "RangeTable", reflect.TypeOf(&unicode.RangeTable{}))
}

// -------- utf8 --------
func registerUTF8() {
	registerPackage("unicode/utf8", map[string]interface{}{
		"DecodeLastRune":    utf8.DecodeLastRune,
		"DecodeLastRuneInString": utf8.DecodeLastRuneInString,
		"DecodeRune":        utf8.DecodeRune,
		"DecodeRuneInString": utf8.DecodeRuneInString,
		"EncodeRune":        utf8.EncodeRune,
		"RuneCount":         utf8.RuneCount,
		"RuneCountInString": utf8.RuneCountInString,
		"RuneLen":           utf8.RuneLen,
		"RuneStart":         utf8.RuneStart,
		"Valid":             utf8.Valid,
		"ValidString":       utf8.ValidString,
		"AppendRune":        utf8.AppendRune,
		"FullRune":          utf8.FullRune,
		"FullRuneInString":  utf8.FullRuneInString,
		"MaxRune":           utf8.MaxRune,
		"RuneError":         utf8.RuneError,
		"UTFMax":            utf8.UTFMax,
	})
}

// -------- utf16 --------
func registerUTF16() {
	registerPackage("unicode/utf16", map[string]interface{}{
		"Decode":          utf16.Decode,
		"DecodeRune":      utf16.DecodeRune,
		"Encode":          utf16.Encode,
		"EncodeRune":      utf16.EncodeRune,
		"IsSurrogate":     utf16.IsSurrogate,
		"Rune":            utf16.DecodeRune,
	})
}

// -------- io/fs --------
func registerFS() {
	registerPackage("io/fs", map[string]interface{}{
		"WalkDir": fs.WalkDir,
		"ReadDir": fs.ReadDir,
		"ReadFile": fs.ReadFile,
		"Sub":     fs.Sub,
		"ValidPath": fs.ValidPath,
		"Glob":    fs.Glob,
	})
	registerType("io/fs", "File", reflect.TypeOf((*fs.File)(nil)).Elem())
	registerType("io/fs", "FileInfo", reflect.TypeOf((*fs.FileInfo)(nil)).Elem())
	registerType("io/fs", "DirEntry", reflect.TypeOf((*fs.DirEntry)(nil)).Elem())
}

// -------- hash --------
func registerHash() {
	// hash package doesn't export New32/New64, only register hash/crc32 and hash/crc64
	registerPackage("hash/crc32", map[string]interface{}{
		"ChecksumIEEE":    crc32.ChecksumIEEE,
		"Checksum":        crc32.Checksum,
		"NewIEEE":         crc32.NewIEEE,
		"New":             crc32.New,
		"MakeTable":       crc32.MakeTable,
		"IEEE":            crc32.IEEETable,
		"Castagnoli": crc32.Castagnoli,
		"Koopman":    crc32.Koopman,
	})
	registerPackage("hash/crc64", map[string]interface{}{
		"Checksum":  crc64.Checksum,
		"New":       crc64.New,
		"MakeTable": crc64.MakeTable,
	})
}

// -------- crypto/hash --------
func registerCryptoHash() {
	registerPackage("crypto/md5", map[string]interface{}{
		"New":     md5.New,
		"Sum":     md5.Sum,
		"Size":    md5.Size,
		"BlockSize": md5.BlockSize,
	})
	registerPackage("crypto/sha1", map[string]interface{}{
		"New":     sha1.New,
		"Sum":     sha1.Sum,
		"Size":    sha1.Size,
		"BlockSize": sha1.BlockSize,
	})
	registerPackage("crypto/sha256", map[string]interface{}{
		"New":     sha256.New,
		"Sum256":  sha256.Sum256,
		"Size":    sha256.Size,
		"BlockSize": sha256.BlockSize,
	})
}

// -------- text/template --------
func registerTextTemplate() {
	registerPackage("text/template", map[string]interface{}{
		"New":     template.New,
		"Must":    template.Must,
		"ParseFiles": template.ParseFiles,
		"ParseGlob":  template.ParseGlob,
	})
	registerType("text/template", "Template", reflect.TypeOf(&template.Template{}))
}

// -------- text/scanner --------
func registerTextScanner() {
	registerPackage("text/scanner", map[string]interface{}{
		"ScanInts":     scanner.ScanInts,
		"ScanFloats":   scanner.ScanFloats,
		"TokenString":  scanner.TokenString,
	})
	registerType("text/scanner", "Scanner", reflect.TypeOf(&scanner.Scanner{}))
}

// -------- text/tabwriter --------
func registerTextTabwriter() {
	registerPackage("text/tabwriter", map[string]interface{}{
		"NewWriter":   tabwriter.NewWriter,
		"FilterHTML":  tabwriter.FilterHTML,
		"StripEscape": tabwriter.StripEscape,
	})
}

// -------- syscall --------
func registerSyscall() {
	registerPackage("syscall", map[string]interface{}{
		"Getpid":  syscall.Getpid,
		"Getppid": syscall.Getppid,
		"Getuid":  syscall.Getuid,
		"Getgid":  syscall.Getgid,
		"Geteuid": syscall.Geteuid,
		"Getegid": syscall.Getegid,
	})
}

// -------- testing --------
func registerTesting() {
	registerPackage("testing", map[string]interface{}{
		"MainStart": testing.MainStart,
		"Init":      testing.Init,
	})
	registerType("testing", "T", reflect.TypeOf(&testing.T{}))
	registerType("testing", "B", reflect.TypeOf(&testing.B{}))
}

// -------- runtime/debug --------
func registerDebug() {
	registerPackage("runtime/debug", map[string]interface{}{
		"FreeOSMemory":   debug.FreeOSMemory,
		"GC":             debug.FreeOSMemory,
		"ReadBuildInfo":  debug.ReadBuildInfo,
		"SetGCPercent":   debug.SetGCPercent,
		"SetMemoryLimit": debug.SetMemoryLimit,
		"SetMaxThreads":  debug.SetMaxThreads,
		"Stack":          debug.Stack,
		"WriteHeapDump":  debug.WriteHeapDump,
	})
}

// -------- slices --------
func registerSlices() {
	// All slices package functions are generic and require type instantiation,
	// so they cannot be registered as interface{} values directly.
	_ = registerPackage // avoid unused warning
}

// -------- helpers --------
func registerType(pkg, name string, t reflect.Type) {
	if _, ok := GoTypeRegistry[pkg]; !ok {
		GoTypeRegistry[pkg] = make(map[string]reflect.Type)
	}
	GoTypeRegistry[pkg][name] = t
}
