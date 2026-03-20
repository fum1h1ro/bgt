# bgt — Board Game Tester

AIを使ったボードゲーム試作・テストプレイ用CLIツール。

## 概要

- ゲームロジックをLuaファイルに記述し、CLIで読み込んで実行する
- Claude APIをプレイヤーとして使用し、テストプレイを自動化する
- 完全情報ゲームを主な対象とする
- Mac/Unix/BSD環境で動作する

## 技術スタック

- **ホスト**: Go
- **ゲームロジック記述**: Lua（`gopher-lua`で組み込み）
- **AIプレイヤー**: Claude API（claude-sonnet-4-20250514）
- **状態永続化**: JSON（カレントディレクトリの`.bgt_state.json`）

## ディレクトリ構成

```
bgt/                        # ツール本体
├── main.go
├── engine/
│   ├── loop.go             # ゲームループ
│   ├── lua.go              # Luaブリッジ
│   └── claude.go           # Claude APIプレイヤー
└── CLAUDE.md

games/                      # ゲームロジック（ユーザーが作成）
├── kaiteitan/
│   └── game.lua
└── my_new_game/
    └── game.lua
```

## セッションファイル

カレントディレクトリに以下のファイルを自動生成・管理する：

| ファイル | 内容 |
|---|---|
| `.bgt_state.json` | 現在のゲーム状態（bgtが自動管理） |
| `.bgt_config.json` | 使用中のgame.luaパスなどの設定 |
| `.bgt_log.jsonl` | 全アクションのログ（1行1JSON） |

## CLIコマンド

固定コマンドは以下のみ。アクション名はLuaの`valid_actions()`が返す内容に依存する。

```bash
bgt init <game.luaのパス>   # ゲームロジックを読み込み、設定を保存
bgt start <人数>            # ゲーム開始。setup()を呼び出しstateを保存
bgt status                  # 現在のstateと取れるアクションを表示
bgt do <action_type> [args] # valid_actions()が返したアクションを実行
bgt ai                      # 現在のプレイヤーをClaudeに判断させる
bgt auto                    # 終了まで全プレイヤーをAIで自動進行
```

### bgt do の動作

1. `.bgt_state.json`を読み込む
2. `_progression`から現在のプレイヤーを取得
3. `valid_actions(state, player_id)`を呼び出す
4. 指定した`action_type`が有効なアクションに含まれるか検証
5. `apply_action(state, action, player_id)`を呼び出す
6. `_progression`を更新（step→turn→round の自動進行）
7. `is_round_over()`でラウンド終了判定、終了時は`on_round_end()`→`is_terminal()`→`on_round_start()`
8. 新しいstateを`.bgt_state.json`に保存

### bgt status の出力例

```
現在のプレイヤー: Player 1
ラウンド: 1  ターン: 1  ステップ: 1
state: {...}
取れるアクション: ["roll-die"]
```

### アクションの例（ゲームによって異なる）

```bash
bgt do roll-die
bgt do take-card
bgt do skip
bgt do move direction=forward
```

## ゲーム進行の4層階層

bgtはゲーム進行を Session > Round > Turn > Step の4層で管理する：

| 層 | 意味 | 進行 |
|---|---|---|
| Session | テストプレイ全体 | `is_terminal` で終了判定（ラウンド終了時にチェック） |
| Round | セッション内の区切り | `is_round_over` で終了判定（毎ステップ後にチェック） |
| Turn | 全プレイヤーが1ステップ終えるまで | Go が自動管理 |
| Step | 各プレイヤーの1アクション | Go が自動管理 |

state には `_progression` キーが自動注入され、`round`, `turn`, `step`, `current_player`, `num_players` を保持する。`visible_state` や `bgt status` 表示時には `_progression` は除外される。

## Lua側のインターフェース

ゲームロジックファイルは以下の関数を実装する（setup, valid_actions, apply_action, is_terminal は必須、その他は任意）：

```lua
-- ゲームの初期状態を返す
-- config: { players = 3 } など
-- current_player / turn はGoが _progression で管理するため返す必要はない
function setup(config)
  return { ...初期state... }
end

-- 指定プレイヤーが取れる行動リストを返す
function valid_actions(state, player_id)
  return {
    { type = "take_card" },
    { type = "skip" },
  }
end

-- アクションを適用した新しいstateを返す（元のstateは変更しない）
-- current_player / turn の更新は不要（Goが自動管理）
function apply_action(state, action, player_id)
  local new_state = deep_copy(state)
  -- ...
  return new_state
end

-- ゲーム終了判定。終了していればwinner等を含むテーブルを、継続中はnilを返す
-- ラウンド終了時にのみ呼び出される
function is_terminal(state)
  return nil  -- or { winner = 1 }
end

-- （任意）ラウンド終了判定。trueを返すとラウンド終了処理が走る
-- 未定義の場合は「1ターン完了（全員1回行動）= ラウンド終了」
function is_round_over(state)
  return nil  -- or true
end

-- （任意）ラウンド開始時に呼び出される（カードを配り直す等）
-- 未定義の場合はstateそのまま
function on_round_start(state, round_number)
  return state
end

-- （任意）ラウンド終了時に呼び出される（スコア集計等）
-- 未定義の場合はstateそのまま
function on_round_end(state)
  return state
end

-- （任意）指定プレイヤーに見える状態を返す
-- bgt status および AI プレイヤーに渡すstateをフィルタリングする
-- 未定義の場合は全stateがそのまま表示される
function visible_state(state, player_id)
  local s = deep_copy(state)
  s.deck = nil  -- 山札は誰にも見えない
  -- 他プレイヤーの手札を隠す
  for _, p in ipairs(s.players) do
    if p.id ~= player_id then
      p.hand = nil
    end
  end
  return s
end

-- （任意）ゲームのルールと現在の状況を自然言語で返す
-- AI プレイヤーのプロンプトに挿入され、判断精度を向上させる
-- 未定義の場合はルール説明なしで動作する
function describe(state, player_id)
  return "ゲームのルール説明..."
end
```

## Go↔Luaブリッジの責務

| 責務 | 担当 |
|---|---|
| ゲームループ管理 | Go |
| プレイヤー交代・ターン/ラウンド進行 | Go（`_progression`） |
| サイコロなどのランダム処理 | Go |
| state の JSON シリアライズ/保存/読み込み | Go |
| Claude APIの呼び出し | Go |
| ゲーム固有のロジック | Lua |

## Claude APIプレイヤーの動作

1. GoがstateをJSONに変換
2. `valid_actions()`の結果とともにClaudeに送信
3. Claudeはアクションを選択してJSONで返答
4. GoがそのアクションでLuaの`apply_action()`を呼び出す

**プロンプトのイメージ：**

```
あなたはボードゲームのプレイヤーです。
勝利を目指して最善の行動を選んでください。

現在の状態：
{...state の JSON...}

取れる行動：
[{"type": "take_card"}, {"type": "skip"}]

選んだ行動をJSONで返してください。例: {"type": "take_card"}
```

## bgt auto の動作

- `is_terminal()`がnilを返す間、ループを継続
- 各ターンのプレイヤーをすべてClaudeが担当
- 結果と全ログを`.bgt_log.jsonl`に保存
- 複数回実行してバランス検証に使用することを想定

## 将来の拡張（現時点では実装不要）

- `bgt run --times 100` — 指定回数の自動実行と統計出力
- ランダムBot / ヒューリスティックBotのサポート
