package flags

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
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
	name   string       // 命令名称
	desc   string       // 命令描述
	params []*param     // 命令参数
	cmds   []*FlagSet   // 子命令
	fn     Handler      // 命令执行代码
	mws    []Middleware // 中间件
	parent *FlagSet     // 父命令
	stmt   *FlagSet
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
		name: name,
		desc: desc,
	}
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
			fmt.Fprintf(w, " %v", p.typ)
			if p.dft != nil {
				if t, ok := p.dft.(time.Time); ok {
					fmt.Fprintf(w, " (default: %q)", t.Format(DateTime))
				} else if s, ok := p.dft.(string); ok {
					fmt.Fprintf(w, " (default: %q)", s)
				} else {
					fmt.Fprintf(w, " (default: %v)", p.dft)
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
			fmt.Fprintf(w, "  %v\n", cmd.name)
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
		desc:   fs.desc,
		params: params,
		mws:    mws,
		parent: fs,
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
	if name == "" {
		panic(fmt.Errorf("flags: subcommand name cannot be empty"))
	}
	for _, cmd := range fs.cmds {
		if cmd.name == name {
			panic(fmt.Errorf("flags: duplicated subcommand: %v", name))
		}
	}

	params := make([]*param, len(fs.params))
	copy(params, fs.params)

	cmd := &FlagSet{
		name:   name,
		desc:   desc,
		params: params,
		mws:    mws,
		parent: fs,
	}
	if fs.stmt != nil {
		fs.stmt.cmds = append(fs.stmt.cmds, cmd)
	} else {
		fs.cmds = append(fs.cmds, cmd)
	}
	return cmd
}

func (fs *FlagSet) addVar(ptr any, shortByte byte, long string, dft any, desc string, seperator ...string) {
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

	if typ := reflect.TypeOf(ptr); typ.Kind() != reflect.Pointer {
		panic(fmt.Errorf("flags: var type %v must be a pointer", typ))
	}

	if dft != nil {
		if dv := reflect.ValueOf(dft); dv.IsZero() {
			dft = nil
		} else {
			t1 := reflect.TypeOf(ptr).Elem()
			t2 := reflect.TypeOf(dft)
			if t1 != t2 {
				panic(fmt.Errorf("flags: var pointer type %v not match default value type %v", t1, t2))
			}
		}
	}

	typ := reflect.TypeOf(ptr).Elem().String()
	switch typ {
	case "time.Duration":
		typ = "duration"
	case "time.Time":
		typ = fmt.Sprintf("datetime, format: %q", DateTime)
	}

	sep1 := ","
	if len(seperator) > 0 && seperator[0] != "" {
		sep1 = seperator[0]
	}
	sep2 := ":"
	if len(seperator) > 1 && seperator[1] != "" {
		sep2 = seperator[1]
	}
	fs.params = append(fs.params, &param{
		ptr:   ptr,
		typ:   typ,
		dft:   dft,
		short: short,
		long:  strings.TrimLeft(long, "-"),
		desc:  desc,
		sep1:  sep1,
		sep2:  sep2,
	})
}

func isNumber(b byte) bool {
	return '0' <= b && b <= '9'
}

func isLetter(b byte) bool {
	return ('A' <= b && b <= 'Z') ||
		('a' <= b && b <= 'z')
}

func isSymbol(b byte) bool {
	return b == '-' || b == '_' || b == '.'
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

func (fs *FlagSet) Int(short byte, long string, dft int, desc string) *int {
	ptr := new(int)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) IntVar(ptr *int, short byte, long string, dft int, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Int8(short byte, long string, dft int8, desc string) *int8 {
	ptr := new(int8)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Int8Var(ptr *int8, short byte, long string, dft int8, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Int16(short byte, long string, dft int16, desc string) *int16 {
	ptr := new(int16)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Int16Var(ptr *int16, short byte, long string, dft int16, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Int32(short byte, long string, dft int32, desc string) *int32 {
	ptr := new(int32)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Int32Var(ptr *int32, short byte, long string, dft int32, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Int64(short byte, long string, dft int64, desc string) *int64 {
	ptr := new(int64)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Int64Var(ptr *int64, short byte, long string, dft int64, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Uint(short byte, long string, dft uint, desc string) *uint {
	ptr := new(uint)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) UintVar(ptr *uint, short byte, long string, dft uint, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Uint8(short byte, long string, dft uint8, desc string) *uint8 {
	ptr := new(uint8)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Uint8Var(ptr *uint8, short byte, long string, dft uint8, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Uint16(short byte, long string, dft uint16, desc string) *uint16 {
	ptr := new(uint16)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Uint16Var(ptr *uint16, short byte, long string, dft uint16, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Uint32(short byte, long string, dft uint32, desc string) *uint32 {
	ptr := new(uint32)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Uint32Var(ptr *uint32, short byte, long string, dft uint32, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Uint64(short byte, long string, dft uint64, desc string) *uint64 {
	ptr := new(uint64)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Uint64Var(ptr *uint64, short byte, long string, dft uint64, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Float32(short byte, long string, dft float32, desc string) *float32 {
	ptr := new(float32)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Float32Var(ptr *float32, short byte, long string, dft float32, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Float64(short byte, long string, dft float64, desc string) *float64 {
	ptr := new(float64)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) Float64Var(ptr *float64, short byte, long string, dft float64, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Str(short byte, long string, dft string, desc string) *string {
	ptr := new(string)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) StrVar(ptr *string, short byte, long string, dft string, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Bool(short byte, long string, dft bool, desc string) *bool {
	ptr := new(bool)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) BoolVar(ptr *bool, short byte, long string, dft bool, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) Duration(short byte, long string, dft time.Duration, desc string) *time.Duration {
	ptr := new(time.Duration)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) DurationVar(ptr *time.Duration, short byte, long string, dft time.Duration, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

func (fs *FlagSet) DateTime(short byte, long string, dft time.Time, desc string) *time.Time {
	ptr := new(time.Time)
	fs.addVar(ptr, short, long, dft, desc)
	return ptr
}

func (fs *FlagSet) DateTimeVar(ptr *time.Time, short byte, long string, dft time.Time, desc string) {
	fs.addVar(ptr, short, long, dft, desc)
}

// AnyVar: add any pointer to parse.
// param ptr must be a pointer,
// param dft should be nil if no default value,
// or else dft type must be reflect.TypeOf(ptr).Elem().
func (fs *FlagSet) AnyVar(ptr any, short byte, long string, dft any, desc string, seperator ...string) {
	fs.addVar(ptr, short, long, dft, desc)
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

func Any[T Types[K, V], K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft T, desc string, seperator ...string) *T {
	ptr := new(T)
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
	return ptr
}

func AnyVar[T Types[K, V], K KeyTypes, V ElemTypes](fs *FlagSet, ptr *T, short byte, long string, dft T, desc string, seperator ...string) {
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
}

func Slice[T ElemTypes](fs *FlagSet, short byte, long string, dft []T, desc string, seperator ...string) *[]T {
	ptr := new([]T)
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
	return ptr
}

func SliceVar[T ElemTypes](fs *FlagSet, ptr *[]T, short byte, long string, dft []T, desc string, seperator ...string) {
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
}

func Map[K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft map[K]V, desc string, seperator ...string) *map[K]V {
	ptr := new(map[K]V)
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
	return ptr
}

func MapVar[K KeyTypes, V ElemTypes](fs *FlagSet, ptr *map[K]V, short byte, long string, dft map[K]V, desc string, seperator ...string) {
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
}

func SliceMap[K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft []map[K]V, desc string, seperator ...string) *[]map[K]V {
	ptr := new([]map[K]V)
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
	return ptr
}

func SliceMapVar[K KeyTypes, V ElemTypes](fs *FlagSet, ptr *[]map[K]V, short byte, long string, dft []map[K]V, desc string, seperator ...string) {
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
}

func MapSlice[K KeyTypes, V ElemTypes](fs *FlagSet, short byte, long string, dft map[K][]V, desc string, seperator ...string) *map[K][]V {
	ptr := new(map[K][]V)
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
	return ptr
}

func MapSliceVar[K KeyTypes, V ElemTypes](fs *FlagSet, ptr *map[K][]V, short byte, long string, dft map[K][]V, desc string, seperator ...string) {
	fs.AnyVar(ptr, short, long, dft, desc, seperator...)
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
		if c.name == arg {
			cmd = c
			break
		}
	}
	if cmd == nil {
		if arg == "help" {
			return fs, ErrHelp
		}
		return fs, fmt.Errorf("%v: unknown sub command: %v", fs.name, arg)
	}
	return cmd._parse(args)
}

func (fs *FlagSet) _parseShort(args *arguments, arg string) error {
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

	if strings.HasPrefix(arg, "--"+param.long+"=") {
		val := strings.TrimPrefix(arg, "--"+param.long+"=")
		return fs._parseParam(newArg(val), arg, param)
	}
	return fs._parseParam(args, arg, param)
}

var (
	typDuration = reflect.TypeOf(time.Duration(0))
	typDateTime = reflect.TypeOf(time.Time{})
)

func (fs *FlagSet) _parseParam(args *arguments, arg string, p *param) error {
	p.parsed = true

	typ := reflect.TypeOf(p.ptr).Elem()
	switch typ {
	case typDuration:
		return fs._parseDuration(args, arg, p)
	case typDateTime:
		return fs._parseDateTime(args, arg, p)
	default:
		switch typ.Kind() {
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
}

func (fs *FlagSet) _parseParamErr(arg string, err error) error {
	return fmt.Errorf("%v: parse option %v: %w", fs.fullName(), arg, err)
}

func (fs *FlagSet) _parseDuration(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	dur, err := time.ParseDuration(args.next())
	if err != nil {
		return fs._parseParamErr(arg, err)
	}
	*p.ptr.(*time.Duration) = dur
	return nil
}

func (fs *FlagSet) _parseDateTime(args *arguments, arg string, p *param) error {
	if args.end() {
		return fs._parseParamErr(arg, ErrNoInputValue)
	}

	t, err := time.ParseInLocation(DateTime, args.next(), time.Local)
	if err != nil {
		return fs._parseParamErr(arg, err)
	}
	*p.ptr.(*time.Time) = t
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
