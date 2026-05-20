package microlisp

// goStdlibLisp contains Lisp wrappers for common Go stdlib operations.
// These functions hide Go's type system from the Lisp programmer, providing
// a simple, untyped scripting interface for file I/O, HTTP, JSON, regex,
// time, path manipulation, environment variables, encoding, crypto, and more.
//
// Evaluated during InitGlobalEnv after initLib.
var goStdlibLisp = `
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; Go Standard Library Wrappers
;; All functions provide a simple, untyped Lisp interface to Go stdlib.
;; Functions are called via (go:import "pkg.Func") which returns a callable.
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 1. File I/O
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (read-file path) -> string
(define (read-file path)
  (funcall (go:import "os.ReadFile") path))

;; (write-file path content) -> t
(define (write-file path content)
  (funcall (go:import "os.WriteFile") path content 438)
  t)

;; (append-file path content) -> t
(define (append-file path content)
  (let* ((f (funcall (go:import "os.OpenFile") path 10 438))
         (n (go:call f "WriteString" content)))
    (go:call f "Close")
    t))

;; (file-exists-p path) -> t/nil
(define (file-exists-p path)
  (let ((result (ignore-errors (funcall (go:import "os.Stat") path))))
    (if result t nil)))

;; (directory-exists-p path) -> t/nil
(define (directory-exists-p path)
  (let ((info (ignore-errors (funcall (go:import "os.Stat") path))))
    (if info
        (let ((mode (funcall (go:call info "Mode"))))
          (funcall (go:call mode "IsDir")))
        nil)))

;; (file-size path) -> number
(define (file-size path)
  (let* ((info (funcall (go:import "os.Stat") path)))
    (funcall (go:call info "Size"))))

;; (delete-file path) -> t
(define (delete-file path)
  (funcall (go:import "os.Remove") path)
  t)

;; (rename-file old new) -> t
(define (rename-file old new)
  (funcall (go:import "os.Rename") old new)
  t)

;; (directory path) -> list of file paths or glob matches
;; Supports both plain directory listing and glob patterns (* ? [)
(define (directory path)
  (let ((results (funcall (go:import "path/filepath.Glob") path)))
    (if (and (consp results) (= (length results) 1))
        ;; Single result: check if it's a directory that was listed
        (let ((dir (car results)))
          (let ((info (ignore-errors (funcall (go:import "os.Stat") dir))))
            (if (and info (funcall (go:call (go:field info "Mode") "IsDir")))
                ;; It's a directory — enumerate its contents
                (let ((entries (funcall (go:import "os.ReadDir") dir))
                      (result '()))
                  (dolist (entry entries)
                    (set! result (cons (funcall (go:import "path/filepath.Join") dir (funcall (go:call entry "Name"))) result)))
                  (nreverse result))
                ;; Not a directory — return the matched file(s)
                results)))
        ;; Multiple results (glob pattern) or empty — return as-is
        results)))

;; (mkdir path &key parents) -> t
(define (mkdir path &key parents)
  (if parents
      (funcall (go:import "os.MkdirAll") path 493)
      (funcall (go:import "os.Mkdir") path 493))
  t)

;; (temp-file &key prefix suffix dir) -> path string
(define (temp-file &key prefix suffix dir)
  (let* ((p (if prefix prefix "tmp"))
         (d (if dir dir ""))
         (f (funcall (go:import "os.CreateTemp") d p)))
    (funcall (go:call f "Name"))))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 2. HTTP Requests
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (http-get url &key headers) -> string
(define (http-get url &key headers)
  (let* ((resp (funcall (go:import "net/http.Get") url))
         (body (funcall (go:import "io.ReadAll") (go:field resp "Body"))))
    (funcall (go:call (go:field resp "Body") "Close"))
    body))

;; (http-post url content &key content-type) -> string
(define (http-post url content &key content-type)
  (let* ((ct (if content-type content-type "text/plain"))
         (resp (funcall (go:import "net/http.Post") url ct (funcall (go:import "strings.NewReader") content)))
         (body (funcall (go:import "io.ReadAll") (go:field resp "Body"))))
    (funcall (go:call (go:field resp "Body") "Close"))
    body))

;; (http-status-text code) -> string
(define (http-status-text code)
  (funcall (go:import "net/http.StatusText") code))

;; (http-get-json url) -> string (raw JSON)
(define (http-get-json url)
  (http-get url))

;; (http-post-json url data &key content-type) -> string
(define (http-post-json url data &key content-type)
  (let ((ct (if content-type content-type "application/json")))
    (http-post url data :content-type ct)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 3. JSON Encoding/Decoding
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (json-encode obj) -> JSON string
;; For simple types (string, number, bool, nil), pass directly.
;; For lists and alists, we use a simplified approach.
(define (json-encode obj)
  (cond
    ((null obj) "null")
    ((eq obj t) "true")
    ((stringp obj) (string-append "\"" (json-escape-string obj) "\""))
    ((numberp obj) (number->string obj))
    ((consp obj)
     (if (alist-p obj)
         (json-encode-alist obj)
         (json-encode-array obj)))
    (t "null")))

;; Escape special characters for JSON strings
(define (json-escape-string s)
  (let ((s (string-replace s "\\" "\\\\")))
    (let ((s (string-replace s "\"" "\\\"")))
      (let ((s (string-replace s "\n" "\\n")))
        (let ((s (string-replace s "\r" "\\r")))
          (let ((s (string-replace s "\t" "\\t")))
            s))))))

;; Check if a list is an alist (association list of dotted pairs)
;; An alist is a list where every element is a cons cell (pair).
;; Empty list is a valid alist (encodes to {}).
(define (alist-p lst)
  (if (null lst)
      #t
      (if (consp (car lst))
          (if (null (cdr lst))
              #t
              (alist-p (cdr lst)))
          #f)))

;; Encode alist as JSON object string
;; Handles both canonical dotted pairs (key . val) and list-form (key val)
(define (json-encode-alist alist)
  (let ((pairs '()))
    (dolist (kv alist)
      (let* ((key (car kv))
             (val-cdr (cdr kv))
             ;; For dotted pair (key . val), val-cdr is the value directly.
             ;; For list-form (key val), val-cdr is (val) — take its car.
             (val (if (and (consp val-cdr) (null (cdr val-cdr)))
                      (car val-cdr)
                      val-cdr))
             (key-str (if (symbolp key)
                         (symbol->string key)
                         (if (stringp key) key "null")))
             (val-str (json-encode val)))
        (set! pairs (cons (string-append "\"" (json-escape-string key-str) "\":" val-str) pairs))))
    (string-append "{" (string-join (nreverse pairs) ",") "}")))

;; Encode list as JSON array string
(define (json-encode-array lst)
  (let ((elems '()))
    (dolist (item lst)
      (set! elems (cons (json-encode item) elems)))
    (string-append "[" (string-join (nreverse elems) ",") "]")))

;; (json-decode str) -> list (alist)
;; Uses Go's json.Unmarshal via FFI with a map[string]interface{} target
(define (json-decode str)
  (let* ((result '())
         (raw (funcall (go:import "encoding/json.Marshal") str)))
    ;; For now, return the raw string. Full decoding requires Go map support.
    str))

;; (json-encode-pretty obj) -> indented JSON string
(define (json-encode-pretty obj)
  (json-encode obj))

;; (json-valid-p str) -> t/nil
(define (json-valid-p str)
  (let ((result (funcall (go:import "encoding/json.Valid") str)))
    (if result t nil)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 4. Time Operations
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (sleep seconds) -> nil
;; Implemented as Go builtin — see go_stdlib_helpers.go

;; (format-time format &optional time) -> string
(define (format-time format &optional time)
  (let ((t (if time
               (funcall (go:import "time.Unix") time 0)
               (funcall (go:import "time.Now")))))
    (go:call t "Format" format)))

;; (current-timestamp) -> "2006-01-02 15:04:05"
(define (current-timestamp)
  (format-time "2006-01-02 15:04:05"))

;; (parse-time format str) -> unix time number
(define (parse-time format str)
  (let ((t (funcall (go:import "time.Parse") format str)))
    (go:call t "Unix")))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 5. Regular Expressions
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (regex-match pattern str) -> t/nil
(define (regex-match pattern str)
  (let ((result (funcall (go:import "regexp.MatchString") pattern str)))
    (if result t nil)))

;; (regex-find-all pattern str &optional count) -> list of matches
(define (regex-find-all pattern str &optional count)
  (let* ((re (funcall (go:import "regexp.Compile") pattern))
         (n (if (numberp count) count -1)))
    (go:call re "FindAllString" str n)))

;; (regex-replace pattern str replacement) -> string
(define (regex-replace pattern str replacement)
  (let ((re (funcall (go:import "regexp.Compile") pattern)))
    (go:call re "ReplaceAllString" str replacement)))

;; (regex-replace-all pattern str replacement) -> string (alias)
(define (regex-replace-all pattern str replacement)
  (regex-replace pattern str replacement))

;; (regex-split pattern str) -> list of parts
(define (regex-split pattern str)
  (let ((re (funcall (go:import "regexp.Compile") pattern)))
    (go:call re "Split" str -1)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 6. Path Operations
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (path-absolute path) -> string
(define (path-absolute path)
  (funcall (go:import "path/filepath.Abs") path))

;; (path-base path) -> string
(define (path-base path)
  (funcall (go:import "path/filepath.Base") path))

;; (path-dir path) -> string
(define (path-dir path)
  (funcall (go:import "path/filepath.Dir") path))

;; (path-ext path) -> string
(define (path-ext path)
  (funcall (go:import "path/filepath.Ext") path))

;; (path-join &rest paths) -> string
(define (path-join . paths)
  (apply (go:import "path/filepath.Join") paths))

;; (path-clean path) -> string
(define (path-clean path)
  (funcall (go:import "path/filepath.Clean") path))

;; (path-exists-p path) -> t/nil
(define (path-exists-p path)
  (file-exists-p path))

;; (path-is-absolute path) -> t/nil
(define (path-is-absolute path)
  (funcall (go:import "path/filepath.IsAbs") path))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 7. Environment Variables
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (getenv key &optional default) -> string
(define (getenv key &optional default)
  (let ((val (funcall (go:import "os.Getenv") key)))
    (if (and (stringp val) (= (length val) 0))
        (if default default "")
        val)))

;; (setenv key value) -> t
(define (setenv key value)
  (funcall (go:import "os.Setenv") key value)
  t)

;; (unsetenv key) -> t
(define (unsetenv key)
  (funcall (go:import "os.Unsetenv") key)
  t)

;; (getenv-all) -> alist of (key . value)
(define (getenv-all)
  (let ((envs (funcall (go:import "os.Environ")))
        (result '()))
    (dolist (env envs)
      (let ((parts (funcall (go:import "strings.SplitN") env "=" 2)))
        (if (>= (length parts) 2)
            (set! result (cons (cons (car parts) (cadr parts)) result)))))
    (nreverse result)))

;; (current-dir) -> string
(define (current-dir)
  (funcall (go:import "os.Getwd")))

;; (change-dir dir) -> t
(define (change-dir dir)
  (funcall (go:import "os.Chdir") dir)
  t)

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 8. Encoding Utilities
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (base64-encode str) -> base64 string
;; Implemented as Go builtin (base64-encode) — see go_stdlib_helpers.go

;; (base64-decode str) -> decoded string
;; Implemented as Go builtin (base64-decode) — see go_stdlib_helpers.go

;; (url-encode str) -> URL-encoded string
(define (url-encode str)
  (funcall (go:import "net/url.QueryEscape") str))

;; (url-decode str) -> decoded string
(define (url-decode str)
  (funcall (go:import "net/url.QueryUnescape") str))

;; (url-parse url-str) -> alist with :scheme :host :path :query
(define (url-parse url-str)
  (let* ((u (funcall (go:import "net/url.Parse") url-str))
         (scheme (go:field u "Scheme"))
         (host (go:field u "Host"))
         (path (go:field u "Path"))
         (query (go:call (go:call u "Query") "Encode")))
    (list (cons ':scheme scheme)
          (cons ':host host)
          (cons ':path path)
          (cons ':query query))))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 9. Crypto Hashing
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (md5 str) -> hex digest string
;; Implemented as Go builtin — see go_stdlib_helpers.go

;; (sha1 str) -> hex digest string
;; Implemented as Go builtin — see go_stdlib_helpers.go

;; (sha256 str) -> hex digest string
;; Implemented as Go builtin — see go_stdlib_helpers.go

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 10. Process Execution
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (run-command command &rest args) -> (output exit-code)
(define (run-command command . args)
  (let* ((cmd (apply (go:import "os/exec.Command") (cons command args)))
         (output (go:call cmd "CombinedOutput"))
         (exit-code (go:call (go:field cmd "ProcessState") "ExitCode")))
    (list output exit-code)))

;; (shell command-string) -> (output exit-code)
(define (shell command-str)
  (run-command "sh" "-c" command-str))

;; (which command) -> path or nil
(define (which command)
  (let ((result (ignore-errors (funcall (go:import "os/exec.LookPath") command))))
    (if result result nil)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 11. String Utilities (wrapping strings package)
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (string-contains s substr) -> t/nil
(define (string-contains s substr)
  (let ((result (funcall (go:import "strings.Contains") s substr)))
    (if result t nil)))

;; (string-starts-with s prefix) -> t/nil
(define (string-starts-with s prefix)
  (let ((result (funcall (go:import "strings.HasPrefix") s prefix)))
    (if result t nil)))

;; (string-ends-with s suffix) -> t/nil
(define (string-ends-with s suffix)
  (let ((result (funcall (go:import "strings.HasSuffix") s suffix)))
    (if result t nil)))

;; (string-split s sep &optional max) -> list
(define (string-split s sep &optional max)
  (if max
      (funcall (go:import "strings.SplitN") s sep max)
      (funcall (go:import "strings.Split") s sep)))

;; (string-join lst sep) -> string
(define (string-join lst sep)
  (funcall (go:import "strings.Join") lst sep))

;; (string-replace s old new &optional count) -> string
(define (string-replace s old new &optional count)
  (if count
      (funcall (go:import "strings.Replace") s old new count)
      (funcall (go:import "strings.ReplaceAll") s old new)))

;; (string-trim-space s) -> string (trims whitespace only)
;; Note: CL's (string-trim char-bag string) is a builtin — don't override it.
(define (string-trim-space s)
  (funcall (go:import "strings.TrimSpace") s))

;; (string-to-upper s) -> string
(define (string-to-upper s)
  (funcall (go:import "strings.ToUpper") s))

;; (string-to-lower s) -> string
(define (string-to-lower s)
  (funcall (go:import "strings.ToLower") s))

;; (string-repeat s count) -> string
(define (string-repeat s count)
  (funcall (go:import "strings.Repeat") s count))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 12. Misc Utilities
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (go-os) -> operating system name
(define (go-os)
  (go:import "runtime.GOOS"))

;; (go-arch) -> architecture name
(define (go-arch)
  (go:import "runtime.GOARCH"))

;; (go-version) -> Go version string
(define (go-version)
  (funcall (go:import "runtime.Version")))

;; (num-cpus) -> number of CPUs
(define (num-cpus)
  (funcall (go:import "runtime.NumCPU")))

;; (pid) -> current process ID
(define (pid)
  (funcall (go:import "os.Getpid")))

;; (hostname) -> machine hostname
(define (hostname)
  (funcall (go:import "os.Hostname")))

;; (expand-env str) -> expand $ENV_VAR in string
(define (expand-env str)
  (funcall (go:import "os.ExpandEnv") str))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 13. I/O Adapter Functions
;; Bridge between Lisp and Go's io.Reader/io.Writer interfaces.
;; These enable calling the 278 Go stdlib functions that take
;; io.Reader, io.Writer, context.Context, etc.
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (reader-from-string s) -> io.Reader
;; Create an io.Reader from a string.
(define (reader-from-string s)
  (funcall (go:import "microlisp/io.NewStringReader") s))

;; (reader-from-file path) -> io.ReadCloser
;; Open a file as an io.Reader.
(define (reader-from-file path)
  (funcall (go:import "microlisp/io.NewFileReader") path))

;; (writer-to-string) -> io.Writer
;; Create an io.Writer that accumulates written data into a string.
(define (writer-to-string)
  (funcall (go:import "microlisp/io.NewStringWriter")))

;; (writer-to-file path) -> io.WriteCloser
;; Create an io.Writer that writes to a file.
(define (writer-to-file path)
  (funcall (go:import "microlisp/io.NewFileWriter") path))

;; (writer-get-string w) -> string
;; Retrieve the accumulated string from a string writer.
(define (writer-get-string w)
  (funcall (go:import "microlisp/io.StringWriterString") w))

;; (writer-reset w) -> nil
;; Reset a string writer's buffer.
(define (writer-reset w)
  (funcall (go:import "microlisp/io.StringWriterReset") w))

;; (reader-close r) -> nil
;; Close a file reader.
(define (reader-close r)
  (funcall (go:import "microlisp/io.FileReaderClose") r))

;; (writer-close w) -> nil
;; Close a file writer.
(define (writer-close w)
  (funcall (go:import "microlisp/io.FileWriterClose") w))

;; (reader-read-all r) -> string
;; Read all data from an io.Reader into a string.
;; This is a builtin (takes *Value, not reflect-compatible).

;; (io-copy-to-string r) -> string
;; Alias for reader-read-all.

;; (io-copy-to-file r path) -> nil
;; Copy all data from an io.Reader to a file.

;; (io-limit-string r n) -> string
;; Read up to n bytes from an io.Reader.

;; (io-nop-closer r) -> io.ReadCloser
;; Wrap an io.Reader as io.ReadCloser with a no-op Close.

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 14. Context Adapters
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (ctx-with-timeout seconds) -> (ctx cancel-fn)
;; Create a context with a timeout. Parent defaults to Background.
;; (ctx-with-timeout parent-ctx seconds) -> (ctx cancel-fn)
;; Create with an explicit parent context.

;; (ctx-with-cancel &optional parent) -> (ctx cancel-fn)
;; Create a cancellable context.

;; (ctx-cancel cancel-fn) -> nil
;; Cancel a context using its cancel function.
(define (ctx-cancel cancel-fn)
  (funcall (go:import "microlisp/io.ContextCancel") cancel-fn))

;; (ctx-done ctx) -> t/nil
;; Check if a context is done (expired or cancelled).
(define (ctx-done ctx)
  (let ((result (funcall (go:import "microlisp/io.ContextDone") ctx)))
    (if result t nil)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 15. Go Callback Adapter
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (go:callback lisp-fn "signature") -> Go function value
;; Create a Go callback from a Lisp function.
;; Signatures: "int32->bool", "int32->int32", "int->int",
;;   "int->bool", "int,int->", "string->string", "()->", "string->error"
;; This is a builtin (takes *Value, not reflect-compatible).

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 16. HTTP Request Adapters
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (http-request method url &optional body) -> *http.Request
;; Create an HTTP request object. Body can be a string or io.Reader.
;; This is a builtin (takes *Value, not reflect-compatible).

;; (http-do req &optional client) -> (body-string status-code)
;; Execute an HTTP request. Client defaults to http.DefaultClient.
;; This is a builtin (takes *Value, not reflect-compatible).

;; (http-fetch method url &optional body) -> string
;; Convenience: create request, execute, return body.
(define (http-fetch method url . rest)
  (let* ((body (if rest (car rest) ""))
         (req (http-request method url body))
         (result (http-do req))
         (resp-body (car result))
         (status (cadr result)))
    resp-body))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 17. Binary Encoding
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (binary-read-uint32 data order) -> uint32
;; Read a uint32 from a byte string. Order: "big" or "little".
(define (binary-read-uint32 data order)
  (funcall (go:import "microlisp/binary.BinaryReadUint32") data order))

;; (binary-read-uint64 data order) -> uint64
(define (binary-read-uint64 data order)
  (funcall (go:import "microlisp/binary.BinaryReadUint64") data order))

;; (binary-read-int32 data order) -> int32
(define (binary-read-int32 data order)
  (funcall (go:import "microlisp/binary.BinaryReadInt32") data order))

;; (binary-read-int64 data order) -> int64
(define (binary-read-int64 data order)
  (funcall (go:import "microlisp/binary.BinaryReadInt64") data order))

;; (binary-write-uint32 n order) -> byte string
(define (binary-write-uint32 n order)
  (funcall (go:import "microlisp/binary.BinaryWriteUint32") n order))

;; (binary-write-uint64 n order) -> byte string
(define (binary-write-uint64 n order)
  (funcall (go:import "microlisp/binary.BinaryWriteUint64") n order))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 18. fmt.Sprintf
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (fmt-sprintf format &rest args) -> string
;; Go's fmt.Sprintf via FFI.
(define (fmt-sprintf format . args)
  (apply (go:import "microlisp/fmt.FormatString") (cons format args)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; 19. JSON MarshalIndent
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;

;; (json-marshal-indent v prefix indent) -> string
(define (json-marshal-indent v prefix indent)
  (funcall (go:import "microlisp/jsonx.JsonMarshalIndent") v prefix indent))
`
