package flags

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 时间参数格式
const DateTime = "2006-01-02T15:04:05"

const (
	NoShort byte   = 0  // 不设置短参数
	NoLong  string = "" // 不设置长参数
)

var (
	ErrNoExecFunc   = errors.New("no exec func")
	ErrNoInputValue = errors.New("no input value")
	ErrHelp         = errors.New("help")
)

// FlagSet提供一组参数解析/命令执行的绑定关系。不可复用，如需要重复解析，需重新生成新的FlagSet。
type FlagSet struct {
	name    string         // 命令名称
	aliases []string       // 别名
	desc    string         // 命令描述
	params  []*param       // 命令参数
	paramOf map[any]*param // 指针->*param
	cmds    []*FlagSet     // 子命令
	fn      Handler        // 命令执行代码
	mws     []Middleware   // 中间件
	parent  *FlagSet       // 父命令
	stmt    *FlagSet
}

// param参数解析
type param struct {
	ptr    any    // 指针，解析到对应变量
	typ    string // 参数类型，用于生成usage
	dft    any    // 默认值，如果没有解析到ptr，则将ptr内容设置为dft
	short  string // 短参数
	long   string // 长参数
	desc   string // 参数描述
	parsed bool   // 是否已解析，用于判断是否将ptr设置为dft

	sep1 string // seperator of every elem, used by slice & map
	sep2 string // seperator of key/value, used by map
}

// New生成一次性解析对象。name：应用名称，desc：应用描述，用于生成usage
func New(name, desc string) *FlagSet {
	return &FlagSet{
		name:    name,
		desc:    desc,
		paramOf: make(map[any]*param),
	}
}

func Cmdline(desc string) *FlagSet {
	return New(filepath.Base(os.Args[0]), desc)
}

type (
	Handler    func(context.Context) // Handler: command handler，执行命令函数
	Middleware func(ctx context.Context, handler Handler)
)

var ctxKey = new(int)

// CurrentCommandUsage：当前命令用法
func CurrentCommandUsage(ctx context.Context) string {
	if cmd := getCmd(ctx); cmd != nil {
		return cmd.Usage()
	}
	return ""
}

// getCmd：在Handler中获取当前子命令
func getCmd(ctx context.Context) *FlagSet {
	cmd, _ := ctx.Value(ctxKey).(*FlagSet)
	return cmd
}

func putCmd(ctx context.Context, cmd *FlagSet) context.Context {
	return context.WithValue(ctx, ctxKey, cmd)
}

// Use：设置中间件，所有以后注册的Handler会用到该中间件
func (fs *FlagSet) Use(mws ...Middleware) *FlagSet {
	fs.mws = append(fs.mws, mws...)
	return fs
}

// Handle：设置Handler，并可以同时设置该handler的中间件
func (fs *FlagSet) Handle(h Handler, mws ...Middleware) {
	h = chain(fs, mws, h)
	for f := fs; f != nil; f = f.parent {
		h = chain(f, f.mws, h)
	}
	fs.fn = h
}

func chain(fs *FlagSet, mws []Middleware, h Handler) Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		n := i
		next := h
		h = func(ctx context.Context) {
			if v := getCmd(ctx); v != fs {
				ctx = putCmd(ctx, fs)
			}
			fs.mws[n](ctx, next)
		}
	}
	return h
}

// Run：解析参数，并调用子命令handler。常见用法为：`fs.Run(context.Background(), os.Args[1:]...)`。
// 返回Usage及错误信息。Usage保持不为空，业务可根据需要判断是否需要展示Usage。
func (fs *FlagSet) Run(ctx context.Context, args ...string) (string, error) {
	f, err := fs.parse(args)
	if err != nil {
		return f.Usage(), err
	}
	if f.fn == nil {
		return f.Usage(), fmt.Errorf("flags: %w of command %v", ErrNoExecFunc, f.fullName())
	}
	f.fn(ctx)
	return f.Usage(), nil
}

func (fs *FlagSet) RunCmdline(ctx context.Context) {
	usage, err := fs.Run(ctx, os.Args[1:]...)
	if err == nil {
		return
	}
	if errors.Is(err, ErrHelp) {
		fmt.Fprintln(os.Stderr, usage)
		return
	}

	if errors.Is(err, ErrNoExecFunc) {
		fmt.Fprintln(os.Stderr, usage)
	} else {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	os.Exit(1)
}

func (fs *FlagSet) fullName() string {
	var names []string
	for f := fs; f != nil; f = f.parent {
		if f.name != "" {
			names = append(names, f.name)
		}
	}
	for i := 0; i < len(names)/2; i++ {
		j := len(names) - 1 - i
		names[i], names[j] = names[j], names[i]
	}
	return strings.Join(names, " ")
}

// Usage：生成help信息。
func (fs *FlagSet) Usage() string {
	w := new(bytes.Buffer)

	name := fs.fullName()
	fmt.Fprintf(w, "%v - %v\n\n", name, fs.desc)

	fmt.Fprintf(w, "Usage:\n")
	fmt.Fprintf(w, "  %v", name)
	if fs.fn != nil && len(fs.params) > 0 {
		if len(fs.cmds) > 0 {
			fmt.Fprintf(w, " [option|command]")
		} else {
			fmt.Fprintf(w, " [option]")
		}
	} else if len(fs.cmds) > 0 {
		fmt.Fprintf(w, " [command]")
	}
	fmt.Fprintf(w, "\n\n")

	if fs.fn != nil && len(fs.params) > 0 {
		fmt.Fprintf(w, "Options:\n")

		index := 0
		for _, p := range fs.params {
			fmt.Fprintf(w, "  ")
			if p.short != "" {
				fmt.Fprintf(w, "-%v", p.short)
			}
			if p.long != "" {
				if p.short != "" {
					fmt.Fprintf(w, ", ")
				}
				fmt.Fprintf(w, "--%v", p.long)
			}
			if p.short == "" && p.long == "" {
				index++
				fmt.Fprintf(w, "$%v", index)
			}
			fmt.Fprintf(w, " %v", p.typ)
			if p.dft != nil {
				if t, ok := p.dft.(Stringer); ok {
					fmt.Fprintf(w, " (default: %v)", t.FlagString())
				} else if s, ok := p.dft.(string); ok {
					fmt.Fprintf(w, " (default: %q)", s)
				} else {
					if typ := reflect.TypeOf(p.dft); typ != nil && (typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map) {
						buf := new(bytes.Buffer)
						enc := json.NewEncoder(buf)
						enc.SetEscapeHTML(false)
						enc.Encode(p.dft)
						fmt.Fprintf(w, " (default: %s)", bytes.TrimSpace(buf.Bytes()))
					} else {
						fmt.Fprintf(w, " (default: %v)", p.dft)
					}
				}
			}
			fmt.Fprintln(w)
			if p.desc != "" {
				for _, line := range strings.Split(p.desc, "\n") {
					fmt.Fprintf(w, "    %v\n", line)
				}
			}
			fmt.Fprintln(w)
		}
	}

	if len(fs.cmds) > 0 {
		fmt.Fprintf(w, "Commands:\n")
		for _, cmd := range fs.cmds {
			if len(cmd.aliases) > 0 {
				fmt.Fprintf(w, "  %v (aliases: %v)\n", cmd.name, strings.Join(cmd.aliases, ", "))
			} else {
				fmt.Fprintf(w, "  %v\n", cmd.name)
			}
			if cmd.desc != "" {
				for _, line := range strings.Split(cmd.desc, "\n") {
					fmt.Fprintf(w, "    %v\n", line)
				}
			}
			fmt.Fprintln(w)
		}
	}

	return string(bytes.TrimSpace(w.Bytes()))
}

// Stmt：开启一个单独的状态，可用于注册特定中间件，不影响Stmt之后的命令。
func (fs *FlagSet) Stmt(mws ...Middleware) *FlagSet {
	params := make([]*param, len(fs.params))
	copy(params, fs.params)

	s := &FlagSet{
		desc:    fs.desc,
		params:  params,
		paramOf: fs.paramOf,
		mws:     mws,
		parent:  fs,
	}
	if fs.stmt != nil {
		s.stmt = fs.stmt
	} else {
		s.stmt = fs
	}
	return s
}

// Cmd：注册子命令，及子命令用到的中间件。
func (fs *FlagSet) Cmd(name, desc string, mws ...Middleware) *FlagSet {
	name = strings.TrimSpace(name)
	if name == "" {
		panic(fmt.Errorf("flags: subcommand name cannot be empty"))
	}
	if strings.HasPrefix(name, "-") {
		panic(fmt.Errorf("flags: subcommand name cannot start with '-': %v", name))
	}
	for _, cmd := range fs.cmds {
		if cmd.name == name || slices.Contains(fs.aliases, name) {
			panic(fmt.Errorf("flags: duplicated subcommand: %v", name))
		}
	}

	params := make([]*param, len(fs.params))
	copy(params, fs.params)

	cmd := &FlagSet{
		name:    name,
		desc:    desc,
		params:  params,
		paramOf: fs.paramOf,
		mws:     mws,
		parent:  fs,
	}
	if fs.stmt != nil {
		fs.stmt.cmds = append(fs.stmt.cmds, cmd)
	} else {
		fs.cmds = append(fs.cmds, cmd)
	}
	return cmd
}

// Alias：设置别名
func (fs *FlagSet) Alias(aliases ...string) *FlagSet {
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if strings.HasPrefix(alias, "-") {
			panic(fmt.Errorf("flags: subcommand alias cannot start with '-': %v", alias))
		}
		for _, cmd := range fs.cmds {
			if cmd.name == alias || slices.Contains(fs.aliases, alias) {
				panic(fmt.Errorf("flags: duplicated subcommand: %v", alias))
			}
		}
		fs.aliases = append(fs.aliases, alias)
	}
	return fs
}

type options struct {
	sliceSep string
	kvSep    string
	zeroDft  bool
}

// Options：设置参数规则。
// 可选值有WithSliceSeperator、WithKeyValueSeperator、WithZeroDefault。
type Options interface {
	apply(opt *options)
}

type optionsFunc func(opt *options)

func (f optionsFunc) apply(opt *options) { f(opt) }

// WithSliceSeperator：数组切分规则，默认为","。
// 用于map结构时，先以slice seperator切分成kv对，再用kv seperator切分key/value值。
func WithSliceSeperator(seperator string) Options {
	return optionsFunc(func(opt *options) {
		opt.sliceSep = seperator
	})
}

// WithKeyValueSeperator：key/value切分规则，默认为":"。
// 应先设置WithSliceSeperator切分为kv对，再用kv seperator切分key/value值。
func WithKeyValueSeperator(seperator string) Options {
	return optionsFunc(func(opt *options) {
		opt.kvSep = seperator
	})
}

// WithZeroDefault：是否显示默认零值。
// 比如int参数为零值0时，将不在help中显示"(default: 0)"。
// 可设置该参数强制显示。
func WithZeroDefault(zero bool) Options {
	return optionsFunc(func(opt *options) {
		opt.zeroDft = zero
	})
}

type (
	// Parser：自定义解析规则。
	Parser interface {
		ParseFlag(string) error
	}
	// Type：自定义显示类型，默认为strings.ToLower(reflect.TypeOf(x).String())。
	Type interface {
		FlagType() string
	}
	// Stringer：自定义显示格式，用于默认值展示(default: show_here)。
	// 如果不定义，默认用fmt.Stringer。
	Stringer interface {
		FlagString() string
	}
)

func (fs *FlagSet) addVar(ptr any, shortByte byte, long string, dft any, desc string, optFns ...Options) {
	opts := new(options)
	for _, opt := range optFns {
		opt.apply(opts)
	}

	var short string
	if shortByte != NoShort {
		if !ValidShort(shortByte) {
			panic(fmt.Errorf("flags: invalid short option: %c", shortByte))
		}
		short = string(shortByte)
	}
	long = strings.TrimLeft(long, "-")
	if !ValidLong(long) {
		panic(fmt.Errorf("flags: invalid long option: %q", long))
	}

	for _, p := range fs.params {
		if short != "" && p.short == short {
			panic(fmt.Errorf("flags: duplicated short option: -%v", short))
		}
		if long != "" && p.long == long {
			panic(fmt.Errorf("flags: duplicated long option: --%v", long))
		}
	}
	if prev, ok := fs.paramOf[ptr]; ok {
		var prevParam, currParam string
		if prev.long != "" {
			prevParam = "--" + prev.long
		} else if prev.short != "" {
			prevParam = "-" + prev.short
		}
		if long != "" {
			currParam = "--" + long
		} else if short != "" {
			currParam = "-" + short
		}
		panic(fmt.Errorf("flags: duplicated option var pointer: %v with %v", currParam, prevParam))
	}

	if typ := reflect.TypeOf(ptr); typ.Kind() != reflect.Pointer {
		panic(fmt.Errorf("flags: var type %v must be a pointer", typ))
	}

	if dft != nil {
		if opts.zeroDft {
			t1 := reflect.TypeOf(ptr).Elem()
			t2 := reflect.TypeOf(dft)
			if t1 != t2 {
				panic(fmt.Errorf("flags: var pointer type %v not match default value type %v", t1, t2))
			}
		} else {
			if dv := reflect.ValueOf(dft); dv.IsZero() {
				dft = nil
			}
		}
	}

	var typ string
	if t, ok := ptr.(Type); ok {
		typ = t.FlagType()
	} else if t, ok := dft.(Type); ok {
		typ = t.FlagType()
	} else {
		typ = reflect.TypeOf(ptr).Elem().String()
	}

	sep1 := ","
	if opts.sliceSep != "" {
		sep1 = opts.sliceSep
	}
	sep2 := ":"
	if opts.kvSep != "" {
		sep2 = opts.kvSep
	}
	fs.params = append(fs.params, &param{
		ptr:   ptr,
		typ:   typ,
		dft:   dft,
		short: short,
		long:  long,
		desc:  desc,
		sep1:  sep1,
		sep2:  sep2,
	})
	fs.paramOf[ptr] = fs.params[len(fs.params)-1]
}

func isNumber(b byte) bool {
	return '0' <= b && b <= '9'
}

func isLetter(b byte) bool {
	return ('A' <= b && b <= 'Z') ||
		('a' <= b && b <= 'z')
}

func isSymbol(b byte) bool {
	return slices.Contains([]byte{'-', '_', '.', ':', '+', '/', '@', '~', '%', '^'}, b)
}

func ValidShort(short byte) bool {
	return short == NoShort || isNumber(short) || isLetter(short)
}

func ValidLong(long string) bool {
	if len(long) == 0 {
		return true
	}
	if valid := isNumber(long[0]) || isLetter(long[0]); !valid {
		return false
	}

	for i := 1; i < len(long); i++ {
		valid := isNumber(long[i]) || isLetter(long[i]) || isSymbol(long[i])
		if !valid {
			return false
		}
	}
	return true
}

func (fs *FlagSet) Int(short byte, long string, dft int, desc string, opts ...Options) *int {
	ptr := new(int)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) IntVar(ptr *int, short byte, long string, dft int, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Int8(short byte, long string, dft int8, desc string, opts ...Options) *int8 {
	ptr := new(int8)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Int8Var(ptr *int8, short byte, long string, dft int8, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Int16(short byte, long string, dft int16, desc string, opts ...Options) *int16 {
	ptr := new(int16)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Int16Var(ptr *int16, short byte, long string, dft int16, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Int32(short byte, long string, dft int32, desc string, opts ...Options) *int32 {
	ptr := new(int32)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Int32Var(ptr *int32, short byte, long string, dft int32, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Int64(short byte, long string, dft int64, desc string, opts ...Options) *int64 {
	ptr := new(int64)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Int64Var(ptr *int64, short byte, long string, dft int64, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Uint(short byte, long string, dft uint, desc string, opts ...Options) *uint {
	ptr := new(uint)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) UintVar(ptr *uint, short byte, long string, dft uint, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Uint8(short byte, long string, dft uint8, desc string, opts ...Options) *uint8 {
	ptr := new(uint8)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Uint8Var(ptr *uint8, short byte, long string, dft uint8, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Uint16(short byte, long string, dft uint16, desc string, opts ...Options) *uint16 {
	ptr := new(uint16)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Uint16Var(ptr *uint16, short byte, long string, dft uint16, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Uint32(short byte, long string, dft uint32, desc string, opts ...Options) *uint32 {
	ptr := new(uint32)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Uint32Var(ptr *uint32, short byte, long string, dft uint32, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Uint64(short byte, long string, dft uint64, desc string, opts ...Options) *uint64 {
	ptr := new(uint64)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Uint64Var(ptr *uint64, short byte, long string, dft uint64, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Float32(short byte, long string, dft float32, desc string, opts ...Options) *float32 {
	ptr := new(float32)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Float32Var(ptr *float32, short byte, long string, dft float32, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Float64(short byte, long string, dft float64, desc string, opts ...Options) *float64 {
	ptr := new(float64)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) Float64Var(ptr *float64, short byte, long string, dft float64, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Str(short byte, long string, dft string, desc string, opts ...Options) *string {
	ptr := new(string)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) StrVar(ptr *string, short byte, long string, dft string, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

func (fs *FlagSet) Bool(short byte, long string, dft bool, desc string, opts ...Options) *bool {
	ptr := new(bool)
	fs.addVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func (fs *FlagSet) BoolVar(ptr *bool, short byte, long string, dft bool, desc string, opts ...Options) {
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

type duration time.Duration

var (
	_ Parser   = (*duration)(nil)
	_ Type     = duration(0)
	_ Stringer = duration(0)
)

func (dur *duration) ParseFlag(s string) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*dur = duration(d)
	return nil
}

func (duration) FlagType() string {
	return "duration"
}

func (dur duration) FlagString() string {
	return time.Duration(dur).String()
}

func (fs *FlagSet) Duration(short byte, long string, dft time.Duration, desc string, opts ...Options) *time.Duration {
	ptr := new(time.Duration)
	fs.addVar((*duration)(ptr), short, long, duration(dft), desc, opts...)
	return ptr
}

func (fs *FlagSet) DurationVar(ptr *time.Duration, short byte, long string, dft time.Duration, desc string, opts ...Options) {
	fs.addVar((*duration)(ptr), short, long, duration(dft), desc, opts...)
}

type datetime time.Time

var (
	_ Parser   = (*datetime)(nil)
	_ Type     = datetime{}
	_ Stringer = datetime{}
)

func (dt *datetime) ParseFlag(s string) error {
	t, err := time.ParseInLocation(DateTime, s, time.Local)
	if err != nil {
		return err
	}
	*dt = datetime(t)
	return nil
}

func (datetime) FlagType() string {
	return "datetime"
}

func (dt datetime) FlagString() string {
	return (time.Time)(dt).Format(DateTime)
}

func (fs *FlagSet) DateTime(short byte, long string, dft time.Time, desc string, opts ...Options) *time.Time {
	ptr := new(time.Time)
	fs.addVar((*datetime)(ptr), short, long, datetime(dft), desc, opts...)
	return ptr
}

func (fs *FlagSet) DateTimeVar(ptr *time.Time, short byte, long string, dft time.Time, desc string, opts ...Options) {
	fs.addVar((*datetime)(ptr), short, long, datetime(dft), desc, opts...)
}

// AnyVar: add any pointer to parse.
// param ptr must be a pointer,
// param dft should be nil if no default value,
// or else dft type must be reflect.TypeOf(ptr).Elem().
func (fs *FlagSet) AnyVar(ptr any, short byte, long string, dft any, desc string, opts ...Options) {
	if p, ok := ptr.(*time.Duration); ok {
		ptr = (*duration)(p)
		if dft != nil {
			dft = duration(dft.(time.Duration))
		}
	} else if p, ok := ptr.(*time.Time); ok {
		ptr = (*datetime)(p)
		if dft != nil {
			dft = datetime(dft.(time.Time))
		}
	}
	fs.addVar(ptr, short, long, dft, desc, opts...)
}

type KeyTypes interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 |
		~bool |
		~string
}

type ElemTypes interface {
	KeyTypes | time.Time
}

type ComTypes[K KeyTypes, V ElemTypes] interface {
	[]V | map[K]V | []map[K]V | map[K][]V
}

type Types[K KeyTypes, V ElemTypes] interface {
	ElemTypes | ComTypes[K, V]
}

func Any[T Types[K, V], K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft T, desc string, opts ...Options) *T {
	ptr := new(T)
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func AnyVar[T Types[K, V], K KeyTypes, V ElemTypes](fs *FlagSet, ptr *T, short byte, long string, dft T, desc string, opts ...Options) {
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
}

func Slice[T ElemTypes](fs *FlagSet, short byte, long string, dft []T, desc string, opts ...Options) *[]T {
	ptr := new([]T)
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func SliceVar[T ElemTypes](fs *FlagSet, ptr *[]T, short byte, long string, dft []T, desc string, opts ...Options) {
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
}

func Map[K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft map[K]V, desc string, opts ...Options) *map[K]V {
	ptr := new(map[K]V)
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func MapVar[K KeyTypes, V ElemTypes](fs *FlagSet, ptr *map[K]V, short byte, long string, dft map[K]V, desc string, opts ...Options) {
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
}

func SliceMap[K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft []map[K]V, desc string, opts ...Options) *[]map[K]V {
	ptr := new([]map[K]V)
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func SliceMapVar[K KeyTypes, V ElemTypes](fs *FlagSet, ptr *[]map[K]V, short byte, long string, dft []map[K]V, desc string, opts ...Options) {
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
}

func MapSlice[K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft map[K][]V, desc string, opts ...Options) *map[K][]V {
	ptr := new(map[K][]V)
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
	return ptr
}

func MapSliceVar[K KeyTypes, V ElemTypes](fs *FlagSet, ptr *map[K][]V, short byte, long string, dft map[K][]V, desc string, opts ...Options) {
	fs.AnyVar(ptr, short, long, dft, desc, opts...)
}

type arguments struct {
	args  []string
	idx   int
	align bool
}

func newArgs(args ...string) *arguments {
	return &arguments{args: args}
}

func newArg(arg string) *arguments {
	return &arguments{args: []string{arg}, align: true}
}

func (s *arguments) end() bool {
	return s.idx >= len(s.args)
}

func (s *arguments) next() string {
	if s.end() {
		return ""
	}
	i := s.idx
	s.idx++
	return s.args[i]
}

// Parsed：判断参数是否被解析。
// 返回false表示未被解析，填充默认值。
func (fs *FlagSet) Parsed(pointer any) bool {
	param := fs.paramOf[pointer]
	return param != nil && param.parsed
}

func (fs *FlagSet) parse(args []string) (*FlagSet, error) {
	return fs._parse(newArgs(args...))
}

func (fs *FlagSet) setDft() {
	for _, p := range fs.params {
		if !p.parsed && p.dft != nil {
			reflect.ValueOf(p.ptr).Elem().Set(reflect.ValueOf(p.dft))
		}
	}
}

func (fs *FlagSet) _parse(args *arguments) (*FlagSet, error) {
	for !args.end() {
		arg := args.next()

		if strings.HasPrefix(arg, "--") {
			if err := fs._parseLong(args, arg); err != nil {
				return fs, err
			}
			continue
		}

		if strings.HasPrefix(arg, "-") {
			if err := fs._parseShort(args, arg); err != nil {
				return fs, err
			}
			continue
		}

		fs.setDft()
		return fs._parseSubcmd(args, arg)
	}

	fs.setDft()
	return fs, nil
}

func (fs *FlagSet) _parseSubcmd(args *arguments, arg string) (*FlagSet, error) {
	var cmd *FlagSet
	for _, c := range fs.cmds {
		if c.name == arg || slices.Contains(c.aliases, arg) {
			cmd = c
			break
		}
	}
	if cmd != nil {
		return cmd._parse(args)
	}

	var index int
	var param *param
	for _, p := range fs.params {
		if p.short == "" && p.long == "" {
			index++
			if !p.parsed {
				param = p
				break
			}
		}
	}
	if param != nil {
		err := fs._parseParam(newArg(arg), fmt.Sprintf("$%v", index), param)
		if err != nil {
			return fs, err
		}
		return fs._parse(args)
	}

	if arg == "help" {
		return fs, ErrHelp
	}
	return fs, fmt.Errorf("%v: unknown sub command: %v", fs.name, arg)
}

func isBoolParam(ptr any) bool {
	_, ok := ptr.(Parser)
	if ok {
		return false
	}
	return reflect.TypeOf(ptr).Elem().Kind() == reflect.Bool
}

func (fs *FlagSet) _parseShort(args *arguments, arg string) error {
	rr := []rune(arg[1:])
	if len(rr) == 0 {
		return fmt.Errorf("%v: unknown option: %v", fs.name, arg)
	}

	if len(rr) == 1 {
		var param *param
		for _, p := range fs.params {
			if p.short != "" && "-"+p.short == arg {
				param = p
				break
			}
		}
		if param == nil {
			if arg == "-h" {
				return ErrHelp
			}
			return fmt.Errorf("%v: unknown option: %v", fs.name, arg)
		}
		return fs._parseParam(args, arg, param)
	}

	var ps []*param
	for _, r := range rr {
		found := false
		for _, p := range fs.params {
			if p.short != "" && p.short == string(r) {
				if isBoolParam(p.ptr) {
					reflect.ValueOf(p.ptr).Elem().SetBool(true)
				} else {
					ps = append(ps, p)
				}
				found = true
				break
			}
		}
		if !found {
			if r == 'h' {
				return ErrHelp
			}
			return fmt.Errorf("%v: unknown option: -%c", fs.name, r)
		}
	}

	slices.Reverse(ps)
	for _, p := range ps {
		err := fs._parseParam(args, arg, p)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fs *FlagSet) _parseLong(args *arguments, arg string) error {
	var param *param
	for _, p := range fs.params {
		if p.long != "" {
			if "--"+p.long == arg {
				param = p
				break
			}
			if strings.HasPrefix(arg, "--"+p.long+"=") {
				param = p
				break
			}
		}
	}
	if param == nil {
		if arg == "--help" {
			return ErrHelp
		}
		return fmt.Errorf("%v: unknown option: %v", fs.name, arg)
	}

	if value, found := strings.CutPrefix(arg, "--"+param.long+"="); found {
		return fs._parseParam(newArg(value), arg, param)
	}
	return fs._parseParam(args, arg, param)
}

func (fs *FlagSet) _parseParam(args *arguments, arg string, p *param) error {
	p.parsed = true

	switch x := p.ptr.(type) {
	case Parser:
		return fs._parseParser(args, arg, x)
	case *time.Duration:
		return fs._parseParser(args, arg, (*duration)(x))
	case *time.Time:
		return fs._parseParser(args, arg, (*datetime)(x))
	}

	switch reflect.TypeOf(p.ptr).Elem().Kind() {
	default:
		return fs._parseParamErr(arg, fmt.Errorf("unsupported type %v", p.typ))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fs._parseInts(args, arg, p)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fs._parseUints(args, arg, p)
	case reflect.Float32:
		return fs._parseFloat32(args, arg, p)
	case reflect.Float64:
		return fs._parseFloat64(args, arg, p)
	case reflect.Bool:
		return fs._parseBool(args, arg, p)
	case reflect.String:
		return fs._parseString(args, arg, p)
	case reflect.Slice:
		if args.align {
			return fs._parseSliceAlign(args, arg, p)
		}
		return fs._parseSlice(args, arg, p)
	case reflect.Map:
		return fs._parseMap(args, arg, p)
	}
}

func (fs *FlagSet) _parseParamErr(arg string, err error) error {
	return fmt.Errorf("%v: parse option %v: %w", fs.fullName(), arg, err)
}

func (fs *FlagSet) _parseParser(args *arguments, arg string, p Parser) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}
	err := p.ParseFlag(args.next())
	if err != nil {
		return fs._parseParamErr(arg, err)
	}
	return nil
}

func (fs *FlagSet) _parseInts(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	i, err := strconv.ParseInt(args.next(), 10, 64)
	if err != nil {
		return fs._parseParamErr(arg, err)
	}

	val := reflect.ValueOf(p.ptr).Elem()
	val.SetInt(i)
	if val.Int() != i {
		return fs._parseParamErr(arg, fmt.Errorf("cannot set %v to an %v, overflowed", i, p.typ))
	}
	return nil
}

func (fs *FlagSet) _parseUints(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	i, err := strconv.ParseUint(args.next(), 10, 64)
	if err != nil {
		return fs._parseParamErr(arg, err)
	}

	val := reflect.ValueOf(p.ptr).Elem()
	val.SetUint(i)
	if val.Uint() != i {
		return fs._parseParamErr(arg, fmt.Errorf("cannot set %v to an %v, overflowed", i, p.typ))
	}
	return nil
}

func (fs *FlagSet) _parseFloat32(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	f, err := strconv.ParseFloat(args.next(), 32)
	if err != nil {
		return fs._parseParamErr(arg, err)
	}
	*p.ptr.(*float32) = float32(f)
	return nil
}

func (fs *FlagSet) _parseFloat64(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	f, err := strconv.ParseFloat(args.next(), 64)
	if err != nil {
		return fs._parseParamErr(arg, err)
	}
	*p.ptr.(*float64) = f
	return nil
}

func (fs *FlagSet) _parseBool(args *arguments, arg string, p *param) error {
	if !args.align {
		*p.ptr.(*bool) = true
		return nil
	}

	s := args.next()
	if s == "true" {
		*p.ptr.(*bool) = true
		return nil
	}
	if s == "false" {
		*p.ptr.(*bool) = false
		return nil
	}

	return fs._parseParamErr(arg, fmt.Errorf("invalid bool value: %q", s))
}

func (fs *FlagSet) _parseString(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}
	*p.ptr.(*string) = args.next()
	return nil
}

func (fs *FlagSet) _parseSlice(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	val := reflect.ValueOf(p.ptr).Elem()
	typ := val.Type().Elem()
	isPtr := typ.Kind() == reflect.Pointer
	if isPtr {
		typ = typ.Elem()
	}

	bak := p.ptr
	defer func() { p.ptr = bak }()

	ptr := reflect.New(typ)
	p.ptr = ptr.Interface()
	err := fs._parseParam(args, arg, p)
	if err != nil {
		return err
	}
	if isPtr {
		val.Set(reflect.Append(val, ptr))
	} else {
		val.Set(reflect.Append(val, ptr.Elem()))
	}
	return nil
}

func (fs *FlagSet) _parseSliceAlign(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	val := reflect.ValueOf(p.ptr).Elem()
	typ := val.Type().Elem()
	if typ.Kind() == reflect.Map {
		return fs._parseSlice(newArgs(args.next()), arg, p)
	}

	isPtr := typ.Kind() == reflect.Pointer
	if isPtr {
		typ = typ.Elem()
	}

	bak := p.ptr
	defer func() { p.ptr = bak }()

	for _, elem := range strings.Split(args.next(), p.sep1) {
		ptr := reflect.New(typ)
		p.ptr = ptr.Interface()
		err := fs._parseParam(newArg(elem), arg, p)
		if err != nil {
			return err
		}
		if isPtr {
			val.Set(reflect.Append(val, ptr))
		} else {
			val.Set(reflect.Append(val, ptr.Elem()))
		}
	}
	return nil
}

func (fs *FlagSet) _parseMap(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}
	s := args.next()
	if s == "" {
		return nil
	}

	val := reflect.ValueOf(p.ptr).Elem()
	typ := val.Type()
	kt := typ.Key()
	vt := typ.Elem()

	for _, pair := range strings.Split(s, p.sep1) {
		kv := strings.Split(pair, p.sep2)
		if len(kv) != 2 {
			return fs._parseParamErr(arg,
				fmt.Errorf("parse key/value: split %q by %q: found %v part(s)", pair, p.sep2, len(kv)),
			)
		}

		k := reflect.New(kt)
		v := reflect.New(vt)

		err := fs._parseParam(
			&arguments{args: []string{kv[0]}},
			arg,
			&param{typ: kt.String(), ptr: k.Interface()},
		)
		if err != nil {
			return err
		}

		err = fs._parseParam(
			&arguments{args: []string{kv[1]}},
			arg,
			&param{typ: vt.String(), ptr: v.Interface()},
		)
		if err != nil {
			return err
		}

		if val.IsNil() {
			val.Set(reflect.MakeMap(typ))
		}
		if vt.Kind() == reflect.Slice {
			if ori := val.MapIndex(k.Elem()); ori.IsValid() {
				val.SetMapIndex(k.Elem(), reflect.AppendSlice(ori, v.Elem()))
			} else {
				val.SetMapIndex(k.Elem(), v.Elem())
			}
		} else {
			val.SetMapIndex(k.Elem(), v.Elem())
		}
	}
	return nil
}
