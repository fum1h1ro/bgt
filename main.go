package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/k-ya/bgt/engine"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	session := engine.NewSession()

	var err error
	switch os.Args[1] {
	case "init":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "使い方: bgt init <game.luaのパス>")
			os.Exit(1)
		}
		err = session.Init(os.Args[2])

	case "start":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "使い方: bgt start <人数> [--seed=N]")
			os.Exit(1)
		}
		numPlayers, seed, parseErr := parseStartArgs(os.Args[2:])
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "引数が不正です: %v\n", parseErr)
			os.Exit(1)
		}
		err = session.Start(numPlayers, seed)

	case "status":
		err = session.Status()

	case "do":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "使い方: bgt do <action_type> [key=value ...]")
			os.Exit(1)
		}
		actionType := os.Args[2]
		args := parseArgs(os.Args[3:])
		err = session.Do(actionType, args)

	case "ai":
		err = session.AI()

	case "auto":
		err = session.Auto()

	default:
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs(rawArgs []string) map[string]string {
	args := make(map[string]string)
	for _, arg := range rawArgs {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			args[parts[0]] = parts[1]
		}
	}
	return args
}

func parseStartArgs(args []string) (numPlayers int, seed int64, err error) {
	seed = engine.NewSeed()
	foundPlayers := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--seed=") {
			seedStr := strings.TrimPrefix(arg, "--seed=")
			seed, err = strconv.ParseInt(seedStr, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("seedが不正です: %s", seedStr)
			}
		} else if !foundPlayers {
			numPlayers, err = strconv.Atoi(arg)
			if err != nil {
				return 0, 0, fmt.Errorf("人数が不正です: %s", arg)
			}
			foundPlayers = true
		}
	}
	if !foundPlayers {
		return 0, 0, fmt.Errorf("人数が指定されていません")
	}
	return numPlayers, seed, nil
}

func printUsage() {
	fmt.Println(`bgt - Board Game Tester

使い方:
  bgt init <game.luaのパス>   ゲームロジックを読み込み
  bgt start <人数> [--seed=N] ゲーム開始（seedで乱数固定）
  bgt status                  現在の状態を表示
  bgt do <action> [args]      アクションを実行
  bgt ai                      AIに1手判断させる
  bgt auto                    終了まで全自動プレイ`)
}
