package microlisp

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"container/heap"
	containerlist "container/list"
	"container/ring"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	crypto_rand "crypto/rand"
	"crypto/rc4"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"database/sql/driver"
	"embed"
	"encoding/ascii85"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"encoding/pem"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"html"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"index/suffixarray"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http/cgi"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/mail"
	"net/netip"
	"net/rpc"
	"net/rpc/jsonrpc"
	"net/smtp"
	"net/textproto"
	"os/user"
	"reflect"
	"sync/atomic"
)

func init() {
	registerArchive()
	registerCompress()
	registerContainer()
	registerCrypto()
	registerDatabase()
	registerEmbed()
	registerEncodingExtra()
	registerGo()
	registerHTML()
	registerImage()
	registerIndex()
	registerMime()
	registerNetExtra()
	registerOSExtra()
	registerReflect()
	registerSyncAtomic()
}

// -------- archive --------
func registerArchive() {
	registerPackage("archive/tar", map[string]interface{}{
		"NewReader": tar.NewReader,
		"NewWriter": tar.NewWriter,
	})
	registerType("archive/tar", "Header", reflect.TypeOf(&tar.Header{}))
	registerType("archive/tar", "Reader", reflect.TypeOf(&tar.Reader{}))
	registerType("archive/tar", "Writer", reflect.TypeOf(&tar.Writer{}))

	registerPackage("archive/zip", map[string]interface{}{
		"OpenReader":   zip.OpenReader,
		"NewReader":    zip.NewReader,
		"NewWriter":    zip.NewWriter,
		"RegisterCompressor":   zip.RegisterCompressor,
		"RegisterDecompressor": zip.RegisterDecompressor,
	})
	registerType("archive/zip", "File", reflect.TypeOf(&zip.File{}))
	registerType("archive/zip", "FileHeader", reflect.TypeOf(&zip.FileHeader{}))
	registerType("archive/zip", "Writer", reflect.TypeOf(&zip.Writer{}))
	registerType("archive/zip", "Reader", reflect.TypeOf(&zip.Reader{}))
	registerType("archive/zip", "ReadCloser", reflect.TypeOf(&zip.ReadCloser{}))
}

// -------- compress --------
func registerCompress() {
	registerPackage("compress/bzip2", map[string]interface{}{
		"NewReader": bzip2.NewReader,
	})
	registerPackage("compress/flate", map[string]interface{}{
		"NewReader":    flate.NewReader,
		"NewReaderDict": flate.NewReaderDict,
		"NewWriter":    flate.NewWriter,
		"NewWriterDict": flate.NewWriterDict,
		"BestSpeed":    flate.BestSpeed,
		"BestCompression": flate.BestCompression,
		"DefaultCompression": flate.DefaultCompression,
		"HuffmanOnly": flate.HuffmanOnly,
		"NoCompression": flate.NoCompression,
	})
	registerPackage("compress/gzip", map[string]interface{}{
		"NewReader":    gzip.NewReader,
		"NewWriter":    gzip.NewWriter,
		"BestSpeed":    gzip.BestSpeed,
		"BestCompression": gzip.BestCompression,
		"DefaultCompression": gzip.DefaultCompression,
		"HuffmanOnly": gzip.HuffmanOnly,
		"NoCompression": gzip.NoCompression,
	})
	registerPackage("compress/lzw", map[string]interface{}{
		"NewReader": lzw.NewReader,
		"NewWriter": lzw.NewWriter,
		"MSB":       lzw.MSB,
		"LSB":       lzw.LSB,
	})
	registerPackage("compress/zlib", map[string]interface{}{
		"NewReader":    zlib.NewReader,
		"NewReaderDict": zlib.NewReaderDict,
		"NewWriter":    zlib.NewWriter,
		"NewWriterLevel": zlib.NewWriterLevel,
		"NewWriterLevelDict": zlib.NewWriterLevelDict,
		"BestSpeed":    zlib.BestSpeed,
		"BestCompression": zlib.BestCompression,
		"DefaultCompression": zlib.DefaultCompression,
		"HuffmanOnly": zlib.HuffmanOnly,
		"NoCompression": zlib.NoCompression,
	})
}

// -------- container --------
func registerContainer() {
	registerPackage("container/heap", map[string]interface{}{
		"Init": heap.Init,
		"Push": heap.Push,
		"Pop":  heap.Pop,
		"Remove": heap.Remove,
		"Fix":  heap.Fix,
	})
	registerPackage("container/list", map[string]interface{}{
		"New": containerlist.New,
	})
	registerType("container/list", "List", reflect.TypeOf(&containerlist.List{}))
	registerType("container/list", "Element", reflect.TypeOf(&containerlist.Element{}))

	registerPackage("container/ring", map[string]interface{}{
		"New": ring.New,
	})
	registerType("container/ring", "Ring", reflect.TypeOf(&ring.Ring{}))
}

// -------- crypto --------
func registerCrypto() {
	registerPackage("crypto/aes", map[string]interface{}{
		"NewCipher": aes.NewCipher,
		"BlockSize": aes.BlockSize,
	})
	registerPackage("crypto/cipher", map[string]interface{}{
		"NewCBCDecrypter": cipher.NewCBCDecrypter,
		"NewCBCEncrypter": cipher.NewCBCEncrypter,
		"NewCFBDecrypter": cipher.NewCFBDecrypter,
		"NewCFBEncrypter": cipher.NewCFBEncrypter,
		"NewCTR":         cipher.NewCTR,
		"NewGCM":         cipher.NewGCM,
		"NewOFB":         cipher.NewOFB,
	})
	registerPackage("crypto/des", map[string]interface{}{
		"NewCipher":     des.NewCipher,
		"NewTripleDESCipher": des.NewTripleDESCipher,
		"BlockSize":     des.BlockSize,
	})
	registerPackage("crypto/ecdsa", map[string]interface{}{
		"GenerateKey": ecdsa.GenerateKey,
		"Sign":        ecdsa.Sign,
		"Verify":      ecdsa.Verify,
		"SignASN1":    ecdsa.SignASN1,
		"VerifyASN1":  ecdsa.VerifyASN1,
	})
	registerPackage("crypto/ed25519", map[string]interface{}{
		"GenerateKey":    ed25519.GenerateKey,
		"NewKeyFromSeed": ed25519.NewKeyFromSeed,
		"Sign":           ed25519.Sign,
		"Verify":         ed25519.Verify,
		"PublicKeySize":  ed25519.PublicKeySize,
		"PrivateKeySize": ed25519.PrivateKeySize,
		"SignatureSize":  ed25519.SignatureSize,
		"SeedSize":       ed25519.SeedSize,
	})
	registerPackage("crypto/elliptic", map[string]interface{}{
		"P224":    elliptic.P224,
		"P256":    elliptic.P256,
		"P384":    elliptic.P384,
		"P521":    elliptic.P521,
		"GenerateKey": elliptic.GenerateKey,
		"Marshal":  elliptic.Marshal,
		"Unmarshal": elliptic.Unmarshal,
	})
	registerPackage("crypto/hmac", map[string]interface{}{
		"New": hmac.New,
		"Equal": hmac.Equal,
	})
	registerPackage("crypto/rand", map[string]interface{}{
		"Read": crypto_rand.Read,
		"Prime": crypto_rand.Prime,
		"Int":  crypto_rand.Int,
	})
	registerPackage("crypto/rc4", map[string]interface{}{
		"NewCipher": rc4.NewCipher,
	})
	registerPackage("crypto/rsa", map[string]interface{}{
		"GenerateKey":         rsa.GenerateKey,
		"GenerateMultiPrimeKey": rsa.GenerateMultiPrimeKey,
		"EncryptPKCS1v15":     rsa.EncryptPKCS1v15,
		"DecryptPKCS1v15":     rsa.DecryptPKCS1v15,
		"SignPKCS1v15":        rsa.SignPKCS1v15,
		"VerifyPKCS1v15":      rsa.VerifyPKCS1v15,
		"EncryptOAEP":         rsa.EncryptOAEP,
		"DecryptOAEP":         rsa.DecryptOAEP,
		"SignPSS":             rsa.SignPSS,
		"VerifyPSS":           rsa.VerifyPSS,
	})
	registerPackage("crypto/sha512", map[string]interface{}{
		"New":       sha512.New,
		"Sum512":    sha512.Sum512,
		"Sum384":    sha512.Sum384,
		"Size":      sha512.Size,
		"Size384":   sha512.Size384,
		"BlockSize": sha512.BlockSize,
	})
	registerPackage("crypto/subtle", map[string]interface{}{
		"ConstantTimeCompare": subtle.ConstantTimeCompare,
		"ConstantTimeSelect":  subtle.ConstantTimeSelect,
		"ConstantTimeByteEq":  subtle.ConstantTimeByteEq,
		"ConstantTimeCopy":    subtle.ConstantTimeCopy,
		"ConstantTimeLessOrEq": subtle.ConstantTimeLessOrEq,
		"XORBytes":            subtle.XORBytes,
	})
}

// -------- database/sql --------
func registerDatabase() {
	registerPackage("database/sql", map[string]interface{}{
		"Open":        sql.Open,
		"OpenDB":      sql.OpenDB,
		"Register":    sql.Register,
		"Drivers":     sql.Drivers,
		"ErrNoRows":   sql.ErrNoRows,
		"ErrTxDone":   sql.ErrTxDone,
		"LevelDefault": sql.LevelDefault,
		"LevelReadUncommitted": sql.LevelReadUncommitted,
		"LevelReadCommitted":   sql.LevelReadCommitted,
		"LevelWriteCommitted":  sql.LevelWriteCommitted,
		"LevelRepeatableRead":  sql.LevelRepeatableRead,
		"LevelSnapshot":        sql.LevelSnapshot,
		"LevelSerializable":    sql.LevelSerializable,
		"LevelLinearizable":    sql.LevelLinearizable,
	})
	registerType("database/sql", "DB", reflect.TypeOf(&sql.DB{}))
	registerType("database/sql", "Rows", reflect.TypeOf(&sql.Rows{}))
	registerType("database/sql", "Tx", reflect.TypeOf(&sql.Tx{}))
	registerType("database/sql", "Stmt", reflect.TypeOf(&sql.Stmt{}))
	registerType("database/sql/driver", "Value", reflect.TypeOf((*driver.Value)(nil)).Elem())
}

// -------- embed --------
func registerEmbed() {
	registerPackage("embed", map[string]interface{}{
		"FS": embed.FS{},
	})
	registerType("embed", "FS", reflect.TypeOf(embed.FS{}))
}

// -------- encoding (extra) --------
func registerEncodingExtra() {
	registerPackage("encoding/ascii85", map[string]interface{}{
		"NewDecoder": ascii85.NewDecoder,
		"NewEncoder": ascii85.NewEncoder,
		"Encode":    ascii85.Encode,
		"Decode":    ascii85.Decode,
		"MaxEncodedLen": ascii85.MaxEncodedLen,
	})
	registerPackage("encoding/binary", map[string]interface{}{
		"Read":       binary.Read,
		"Write":      binary.Write,
		"PutUvarint": binary.PutUvarint,
		"PutVarint":  binary.PutVarint,
		"Uvarint":    binary.Uvarint,
		"Varint":     binary.Varint,
		"ReadUvarint": binary.ReadUvarint,
		"ReadVarint":  binary.ReadVarint,
		"Size":       binary.Size,
		"BigEndian":  binary.BigEndian,
		"LittleEndian": binary.LittleEndian,
		"NativeEndian": binary.NativeEndian,
		"MaxVarintLen16": binary.MaxVarintLen16,
		"MaxVarintLen32": binary.MaxVarintLen32,
		"MaxVarintLen64": binary.MaxVarintLen64,
	})
	registerPackage("encoding/gob", map[string]interface{}{
		"NewDecoder": gob.NewDecoder,
		"NewEncoder": gob.NewEncoder,
		"Register":   gob.Register,
		"RegisterName": gob.RegisterName,
	})
	registerPackage("encoding/hex", map[string]interface{}{
		"EncodeToString": hex.EncodeToString,
		"DecodeString":   hex.DecodeString,
		"Encode":         hex.Encode,
		"Decode":         hex.Decode,
		"NewDecoder":     hex.NewDecoder,
		"NewEncoder":     hex.NewEncoder,
		"Dump":           hex.Dump,
		"Dumper":         hex.Dumper,
		"ErrLength":      hex.ErrLength,
	})
	registerPackage("encoding/pem", map[string]interface{}{
		"Encode":   pem.Encode,
		"EncodeToMemory": pem.EncodeToMemory,
		"Decode":   pem.Decode,
	})
	registerType("encoding/pem", "Block", reflect.TypeOf(&pem.Block{}))
}

// -------- go --------
func registerGo() {
	registerPackage("go/ast", map[string]interface{}{
		"NewScope":      ast.NewScope,
		"NewIdent":      ast.NewIdent,
		"NewObj":        ast.NewObj,
		"IsExported":    ast.IsExported,
		"MergePackageFiles": ast.MergePackageFiles,
	})
	registerPackage("go/build", map[string]interface{}{
		"Default": build.Default,
		"Import":  build.Import,
		"ImportDir": build.ImportDir,
	})
	registerPackage("go/format", map[string]interface{}{
		"Node":    format.Node,
		"Source":  format.Source,
	})
	registerPackage("go/parser", map[string]interface{}{
		"ParseFile":  parser.ParseFile,
		"ParseDir":   parser.ParseDir,
		"ParseExpr":  parser.ParseExpr,
		"PackageClauseOnly": parser.PackageClauseOnly,
		"ImportsOnly": parser.ImportsOnly,
		"ParseComments": parser.ParseComments,
		"Trace":       parser.Trace,
		"DeclarationErrors": parser.DeclarationErrors,
		"AllErrors":   parser.AllErrors,
	})
	registerPackage("go/printer", map[string]interface{}{
		"Fprint":  printer.Fprint,
		"RawFormat": printer.RawFormat,
	})
	registerPackage("go/token", map[string]interface{}{
		"NewFileSet": token.NewFileSet,
		"Lookup":     token.Lookup,
		"IsKeyword":  token.IsKeyword,
		"IsIdentifier": token.IsIdentifier,
		"IsExported": token.IsExported,
	})
	registerType("go/token", "FileSet", reflect.TypeOf(&token.FileSet{}))
	registerType("go/token", "Pos", reflect.TypeOf(token.Pos(0)))
	registerType("go/ast", "File", reflect.TypeOf(&ast.File{}))
	registerType("go/ast", "Ident", reflect.TypeOf(&ast.Ident{}))
}

// -------- html --------
func registerHTML() {
	registerPackage("html", map[string]interface{}{
		"EscapeString": html.EscapeString,
		"UnescapeString": html.UnescapeString,
	})
	registerPackage("html/template", map[string]interface{}{
		"New":      template.New,
		"Must":     template.Must,
		"ParseFiles": template.ParseFiles,
		"ParseGlob":  template.ParseGlob,
		"HTMLEscape": template.HTMLEscape,
	})
}

// -------- image --------
func registerImage() {
	registerPackage("image", map[string]interface{}{
		"NewRGBA":  image.NewRGBA,
		"NewNRGBA": image.NewNRGBA,
		"NewAlpha": image.NewAlpha,
		"NewGray":  image.NewGray,
		"NewPaletted": image.NewPaletted,
		"NewUniform":  image.NewUniform,
		"Decode":   image.Decode,
		"DecodeConfig": image.DecodeConfig,
		"RegisterFormat": image.RegisterFormat,
		"Pt":       image.Pt,
		"Rect":     image.Rect,
	})
	registerType("image", "Image", reflect.TypeOf((*image.Image)(nil)).Elem())
	registerType("image", "RGBA", reflect.TypeOf(&image.RGBA{}))
	registerType("image", "Point", reflect.TypeOf(image.Pt(0, 0)))
	registerType("image", "Rectangle", reflect.TypeOf(image.Rect(0, 0, 1, 1)))
	registerType("image", "NRGBA", reflect.TypeOf(&image.NRGBA{}))
	registerType("image", "Uniform", reflect.TypeOf(&image.Uniform{}))

	registerPackage("image/color", map[string]interface{}{
		"RGBAModel":  color.RGBAModel,
		"RGBA64Model": color.RGBA64Model,
		"NRGBAModel": color.NRGBAModel,
		"NRGBA64Model": color.NRGBA64Model,
		"AlphaModel": color.AlphaModel,
		"Alpha16Model": color.Alpha16Model,
		"GrayModel":  color.GrayModel,
		"Gray16Model": color.Gray16Model,
		"CMYKModel":  color.CMYKModel,
		"Black":      color.Black,
		"White":      color.White,
		"Transparent": color.Transparent,
		"Opaque":     color.Opaque,
	})
	registerType("image/color", "Color", reflect.TypeOf((*color.Color)(nil)).Elem())
	registerType("image/color", "RGBA", reflect.TypeOf(color.RGBA{}))
	registerType("image/color", "NRGBA", reflect.TypeOf(color.NRGBA{}))
	registerType("image/color", "Gray", reflect.TypeOf(color.Gray{}))
	registerType("image/color", "CMYK", reflect.TypeOf(color.CMYK{}))
	registerType("image/color", "Model", reflect.TypeOf((*color.Model)(nil)).Elem())
	registerType("image/color", "Palette", reflect.TypeOf(color.Palette{}))

	registerPackage("image/draw", map[string]interface{}{
		"Draw":     draw.Draw,
		"DrawMask": draw.DrawMask,
	})
	registerType("image/draw", "Drawer", reflect.TypeOf((*draw.Drawer)(nil)).Elem())
	registerType("image/draw", "Quantizer", reflect.TypeOf((*draw.Quantizer)(nil)).Elem())

	registerPackage("image/gif", map[string]interface{}{
		"Decode":       gif.Decode,
		"DecodeAll":    gif.DecodeAll,
		"DecodeConfig": gif.DecodeConfig,
		"Encode":       gif.Encode,
		"EncodeAll":    gif.EncodeAll,
	})
	registerPackage("image/jpeg", map[string]interface{}{
		"Decode":       jpeg.Decode,
		"DecodeConfig": jpeg.DecodeConfig,
		"Encode":       jpeg.Encode,
		"DefaultQuality": jpeg.DefaultQuality,
	})
	registerPackage("image/png", map[string]interface{}{
		"Decode":       png.Decode,
		"DecodeConfig": png.DecodeConfig,
		"Encode":       png.Encode,
		"BestCompression": png.BestCompression,
		"BestSpeed":    png.BestSpeed,
		"DefaultCompression": png.DefaultCompression,
		"NoCompression": png.NoCompression,
	})
}

// -------- index --------
func registerIndex() {
	registerPackage("index/suffixarray", map[string]interface{}{
		"New": suffixarray.New,
	})
	registerType("index/suffixarray", "Index", reflect.TypeOf(&suffixarray.Index{}))
}

// -------- mime --------
func registerMime() {
	registerPackage("mime", map[string]interface{}{
		"FormatMediaType":  mime.FormatMediaType,
		"ParseMediaType":   mime.ParseMediaType,
		"ExtensionsByType": mime.ExtensionsByType,
		"TypeByExtension":  mime.TypeByExtension,
		"AddExtensionType": mime.AddExtensionType,
		"BEncoding":        mime.BEncoding,
		"QEncoding":        mime.QEncoding,
	})
	registerPackage("mime/multipart", map[string]interface{}{
		"NewReader": multipart.NewReader,
		"NewWriter": multipart.NewWriter,
	})
	registerType("mime/multipart", "Writer", reflect.TypeOf(&multipart.Writer{}))
	registerType("mime/multipart", "Reader", reflect.TypeOf((*multipart.Reader)(nil)).Elem())
	registerType("mime/multipart", "Part", reflect.TypeOf(&multipart.Part{}))

	registerPackage("mime/quotedprintable", map[string]interface{}{
		"NewReader": quotedprintable.NewReader,
		"NewWriter": quotedprintable.NewWriter,
	})
}

// -------- net (extra) --------
func registerNetExtra() {
	registerPackage("net/http/cgi", map[string]interface{}{
		"Serve":    cgi.Serve,
		"Request":  cgi.Request,
		"RequestFromMap": cgi.RequestFromMap,
	})
	registerPackage("net/http/cookiejar", map[string]interface{}{
		"New": cookiejar.New,
	})
	registerPackage("net/http/httptest", map[string]interface{}{
		"NewServer":     httptest.NewServer,
		"NewTLSServer":  httptest.NewTLSServer,
		"NewRecorder":   httptest.NewRecorder,
	})
	registerType("net/http/httptest", "Server", reflect.TypeOf(&httptest.Server{}))
	registerType("net/http/httptest", "ResponseRecorder", reflect.TypeOf(&httptest.ResponseRecorder{}))

	registerPackage("net/http/httputil", map[string]interface{}{
		"NewSingleHostReverseProxy": httputil.NewSingleHostReverseProxy,
		"DumpRequest":    httputil.DumpRequest,
		"DumpRequestOut": httputil.DumpRequestOut,
		"DumpResponse":   httputil.DumpResponse,
	})
	registerType("net/http/httputil", "ReverseProxy", reflect.TypeOf(&httputil.ReverseProxy{}))

	registerPackage("net/mail", map[string]interface{}{
		"ParseAddress":    mail.ParseAddress,
		"ParseAddressList": mail.ParseAddressList,
		"ReadMessage":     mail.ReadMessage,
	})
	registerType("net/mail", "Address", reflect.TypeOf(&mail.Address{}))
	registerType("net/mail", "Header", reflect.TypeOf(mail.Header{}))

	registerPackage("net/netip", map[string]interface{}{
		"AddrFrom4":      netip.AddrFrom4,
		"AddrFrom16":     netip.AddrFrom16,
		"AddrFromSlice":  netip.AddrFromSlice,
		"IPv4Unspecified": netip.IPv4Unspecified,
		"IPv6LinkLocalAllNodes": netip.IPv6LinkLocalAllNodes,
		"IPv6Loopback":   netip.IPv6Loopback,
		"IPv6Unspecified": netip.IPv6Unspecified,
		"ParseAddr":      netip.ParseAddr,
		"ParseAddrPort":  netip.ParseAddrPort,
		"MustParseAddr":  netip.MustParseAddr,
		"PrefixFrom":     netip.PrefixFrom,
	})
	registerType("net/netip", "Addr", reflect.TypeOf(netip.Addr{}))
	registerType("net/netip", "AddrPort", reflect.TypeOf(netip.AddrPort{}))
	registerType("net/netip", "Prefix", reflect.TypeOf(netip.Prefix{}))

	registerPackage("net/rpc", map[string]interface{}{
		"NewClient":   rpc.NewClient,
		"Dial":        rpc.Dial,
		"Register":    rpc.Register,
		"RegisterName": rpc.RegisterName,
		"Accept":      rpc.Accept,
		"HandleHTTP":  rpc.HandleHTTP,
		"DefaultServer": rpc.DefaultServer,
	})
	registerType("net/rpc", "Client", reflect.TypeOf(&rpc.Client{}))
	registerType("net/rpc", "Server", reflect.TypeOf(&rpc.Server{}))

	registerPackage("net/rpc/jsonrpc", map[string]interface{}{
		"NewClient":     jsonrpc.NewClient,
		"NewServerCodec": jsonrpc.NewServerCodec,
		"Dial":          jsonrpc.Dial,
	})

	registerPackage("net/smtp", map[string]interface{}{
		"NewClient": smtp.NewClient,
		"SendMail":   smtp.SendMail,
		"CRAMMD5Auth":  smtp.CRAMMD5Auth,
		"PlainAuth":  smtp.PlainAuth,
	})

	registerPackage("net/textproto", map[string]interface{}{
		"NewConn":      textproto.NewConn,
		"NewReader":    textproto.NewReader,
		"CanonicalMIMEHeaderKey": textproto.CanonicalMIMEHeaderKey,
	})
}

// -------- os (extra) --------
func registerOSExtra() {
	registerPackage("os/user", map[string]interface{}{
		"Current":  user.Current,
		"Lookup":   user.Lookup,
		"LookupId": user.LookupId,
		"LookupGroup": user.LookupGroup,
		"LookupGroupId": user.LookupGroupId,
	})
	registerType("os/user", "User", reflect.TypeOf(&user.User{}))
	registerType("os/user", "Group", reflect.TypeOf(&user.Group{}))
}

// -------- reflect --------
func registerReflect() {
	registerPackage("reflect", map[string]interface{}{
		"TypeOf":       reflect.TypeOf,
		"ValueOf":      reflect.ValueOf,
		"New":          reflect.New,
		"Zero":         reflect.Zero,
		"MakeSlice":    reflect.MakeSlice,
		"MakeMap":      reflect.MakeMap,
		"MakeChan":     reflect.MakeChan,
		"MakeFunc":     reflect.MakeFunc,
		"Append":       reflect.Append,
		"Copy":         reflect.Copy,
		"DeepEqual":    reflect.DeepEqual,
		"Swapper":      reflect.Swapper,
		"ArrayOf":      reflect.ArrayOf,
		"SliceOf":      reflect.SliceOf,
		"ChanOf":       reflect.ChanOf,
		"MapOf":        reflect.MapOf,
		"PtrTo":        reflect.PtrTo,
		"FuncOf":       reflect.FuncOf,
		"StructOf":     reflect.StructOf,
		"Select":       reflect.Select,
	})
}

// -------- sync/atomic --------
func registerSyncAtomic() {
	registerPackage("sync/atomic", map[string]interface{}{
		"AddInt32":    atomic.AddInt32,
		"AddInt64":    atomic.AddInt64,
		"AddUint32":   atomic.AddUint32,
		"AddUint64":   atomic.AddUint64,
		"AddUintptr":  atomic.AddUintptr,
		"CompareAndSwapInt32": atomic.CompareAndSwapInt32,
		"CompareAndSwapInt64": atomic.CompareAndSwapInt64,
		"CompareAndSwapUint32": atomic.CompareAndSwapUint32,
		"CompareAndSwapUint64": atomic.CompareAndSwapUint64,
		"LoadInt32":   atomic.LoadInt32,
		"LoadInt64":   atomic.LoadInt64,
		"LoadUint32":  atomic.LoadUint32,
		"LoadUint64":  atomic.LoadUint64,
		"LoadPointer": atomic.LoadPointer,
		"StoreInt32":  atomic.StoreInt32,
		"StoreInt64":  atomic.StoreInt64,
		"StoreUint32": atomic.StoreUint32,
		"StoreUint64": atomic.StoreUint64,
		"SwapInt32":   atomic.SwapInt32,
		"SwapInt64":   atomic.SwapInt64,
		"SwapUint32":  atomic.SwapUint32,
		"SwapUint64":  atomic.SwapUint64,
	})
}
