# flags

flags提供了一种命令行参数解析方式。该库设计灵感来源于标准库net/http以及一些三方框架，如[echo](https://github.com/labstack/echo)。该库将命令行参数分为两种：命令和参数。

- 命令：不带`-`开头的参数；
- 参数：以`-`开头的参数。

示例：

```bash
your_app_name draw --shape circle
```

- your_app_name：应用名称；
- draw：命令名称；
- --shape：参数，参数值为"circle"。



## Features

**支持参数类型**：`(u)int(8|16|32|64)`、`float(32|64)`、`string`、`bool`、`time.Duration`、`time.Time`，以及有限的`map`和`slice`。

**中间件**：可以像http中间件一样，自定义中间件，在命令前后执行特定逻辑。

**状态空间**：类似命名空间，为一些命令单独开辟一个状态空间，用于注册中间件等逻辑，不影响之后命令的中间件注册。

**自动生成帮助文档**：根据参数和命令注册顺序，自动生成对应文档，可以根据`-h`或`--help`来查看。



## 用法

```go
// test.go
package main

import (
	"context"
	"fmt"

	"codeberg.org/3w/flags"
)

func main() {
	fs := flags.Cmdline("this is a test app desc")

	bar := fs.Cmd("bar", "the first sub command", func(ctx context.Context, handler flags.Handler) {
		fmt.Printf("before bar\n")
		handler(ctx)
		fmt.Printf("bar quit\n")
	})
	bar.Handle(func(context.Context) {
		fmt.Println("bar")
	})

	file := fs.Str('c', "config", "app.cfg", "config file")

	fs.Use(func(ctx context.Context, handler flags.Handler) {
		fmt.Printf("config file: %v\n", *file)
		handler(ctx)
		fmt.Println("main quit")
	})

	fs.Handle(func(context.Context) {
		fmt.Println("handler")
	})

	foo := fs.Cmd("foo", "the second sub command", func(ctx context.Context, handler flags.Handler) {
		fmt.Printf("before foo, config file: %v\n", *file)
		handler(ctx)
		fmt.Printf("foo quit\n")
	})
	foo.Handle(func(context.Context) {
		fmt.Println("foo")
	})

	fs.RunCmdline(context.Background())
}
```

执行`go run test.go -h`：

```bash
$ go run test.go -h
test - this is a test app desc

Usage:
  test [option|command]

Options:
  -c, --config string (default: "app.cfg")
    config file

Commands:
  bar
    the first sub command

  foo
    the second sub command
```

执行`go run test.go`：

```bash
$ go run test.go
config file: app.cfg
handler
quit
```

执行`go run test.go bar -h`：

```bash
$ go run test.go bar -h
test bar - the first sub command

Usage:
  test bar
```

执行`go run test.go bar`：

```bash
before bar
bar
bar quit
```

执行`go run test.go foo -h`：

```bash
$ go run test.go foo -h
test foo - a sub command

Usage:
  test foo [option]

Options:
  -c, --config string (default: "app.cfg")
    config file
```

执行`go run test.go foo`：

```bash
config file: app.cfg
before foo, config file: app.cfg
foo
foo quit
main quit
```



执行`go run test.go bar -c app.cfg`：

```bash
$ go run test.go bar -c config
bar: unknown option: -c
```

执行`go run test.go -c app.cfg bar`（参数`-c`无效）：

```bash
$ go run test.go -c app.cfg bar
before bar
bar
bar quit
```

执行`go run test.go -c app.cfg foo -c app.json`（`-c`参数值以最后一个为准）：

```bash
$ go run test.go -c app.cfg foo -c app.json
config file: app.json
before foo, config file: app.json
foo
foo quit
main quit
```



