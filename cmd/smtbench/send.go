package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"smtbench/internal/client"
	"smtbench/internal/result"
)

func cmdSend(args []string) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	to := fs.String("to", "", "адрес коллектора, например http://10.0.0.5:8080")
	var file string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		file, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if file == "" && fs.NArg() > 0 {
		file = fs.Arg(0)
	}
	if file == "" || *to == "" {
		fmt.Fprintln(os.Stderr, "использование: smtbench send FILE --to URL")
		return 1
	}
	run, err := result.Load(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return 1
	}
	raw, _ := json.Marshal(run)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	url, err := client.Send(ctx, *to, raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Не удалось отправить:", err)
		return 2
	}
	fmt.Println("Отправлено:", url)
	return 0
}
