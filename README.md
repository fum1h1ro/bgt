# bgt — Board Game Tester

AIを使ったボードゲーム試作・テストプレイ用CLIツール。

ゲームロジックをLuaで書き、CLIから手動プレイ or Claude APIで自動テストプレイできる。

## セットアップ

```bash
go build -o bgt .
```

AI機能（`bgt ai` / `bgt auto`）を使う場合は環境変数を設定：

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

## 使い方

```bash
bgt init games/kaiteitan/game.lua   # ゲームを読み込み
bgt start 3                         # 3人で開始
bgt status                          # 状態を確認
bgt do roll_die                     # アクションを実行
bgt do move direction=forward       # 引数付きアクション
bgt ai                              # 現在のプレイヤーをAIに判断させる
bgt auto                            # 終了まで全自動プレイ
```

## game.lua の書き方

4つの関数を実装するだけでゲームが動く。

### 必須関数

#### `setup(config) → state`

ゲームの初期状態を返す。`config.players` にプレイヤー人数が渡される。

返すstateには **`current_player`**（現在のプレイヤーID、1始まり）と **`turn`**（ターン番号、1始まり）と **`players`**（プレイヤーの配列）を含めること。

```lua
function setup(config)
  local players = {}
  for i = 1, config.players do
    players[i] = { id = i, score = 0 }
  end
  return {
    players = players,
    current_player = 1,
    turn = 1,
    -- ゲーム固有のフィールドを自由に追加
  }
end
```

#### `valid_actions(state, player_id) → actions`

指定プレイヤーが取れるアクションの配列を返す。各アクションは `type` フィールドを持つテーブル。

```lua
function valid_actions(state, player_id)
  local actions = {}
  if state.can_draw then
    table.insert(actions, { type = "draw_card" })
  end
  table.insert(actions, { type = "pass" })
  return actions
end
```

`type` 以外のフィールドも自由に追加できる。CLIでは `bgt do <type> [key=value ...]` で指定する。

#### `apply_action(state, action, player_id) → new_state`

アクションを適用して新しいstateを返す。**元のstateは変更しないこと**（`deep_copy` を使う）。

ターン管理（`current_player` の更新、`turn` のインクリメント）もこの関数内で行う。

```lua
function apply_action(state, action, player_id)
  local new_state = deep_copy(state)

  if action.type == "draw_card" then
    -- ゲーム固有の処理
  end

  -- 次のプレイヤーに交代
  new_state.current_player = (player_id % #new_state.players) + 1
  new_state.turn = new_state.turn + 1

  return new_state
end
```

#### `is_terminal(state) → result or nil`

ゲームが終了していれば結果テーブルを、継続中なら `nil` を返す。

```lua
function is_terminal(state)
  for _, p in ipairs(state.players) do
    if p.score >= 10 then
      return { winner = p.id }
    end
  end
  return nil
end
```

結果テーブルの形式は自由（`winner`、`draw`、`ranking` など）。

### stateの規約

| フィールド | 型 | 説明 |
|---|---|---|
| `current_player` | number | 現在のプレイヤーID（1始まり）。**必須** |
| `turn` | number | ターン連番（1始まり）。**必須** |
| `players` | table(配列) | プレイヤー情報の配列。**必須** |
| その他 | 任意 | ゲーム固有の状態を自由に追加 |

`turn` はログの `turn` フィールド（`ラウンド-ステップ` 形式）の算出に使われる。全プレイヤーが1回ずつプレイすると1ラウンド。

### Go側から提供される関数

#### `deep_copy(table) → table`

テーブルを再帰的にディープコピーする。`apply_action` の冒頭で使う。

```lua
local new_state = deep_copy(state)
```

#### `bgt.roll(sides) → number`

1〜sides のランダムな整数を返す。サイコロに使う。

```lua
local roll = bgt.roll(6)    -- 1〜6
```

#### `bgt.random(min, max) → number`

min〜max のランダムな整数を返す。

```lua
local n = bgt.random(1, 10)  -- 1〜10
```

## 完全な例

```lua
-- シンプルなすごろく: ゴール（マス20）に最初にたどり着いたら勝ち

function setup(config)
  local players = {}
  for i = 1, config.players do
    players[i] = { id = i, position = 0 }
  end
  return {
    players = players,
    current_player = 1,
    goal = 20,
    turn = 1,
  }
end

function valid_actions(state, player_id)
  return {
    { type = "roll_die" }
  }
end

function apply_action(state, action, player_id)
  local new_state = deep_copy(state)
  if action.type == "roll_die" then
    local roll = bgt.roll(6)
    local p = new_state.players[player_id]
    p.position = p.position + roll
    if p.position > new_state.goal then
      p.position = new_state.goal
    end
    new_state.current_player = (player_id % #new_state.players) + 1
    new_state.turn = new_state.turn + 1
  end
  return new_state
end

function is_terminal(state)
  for _, p in ipairs(state.players) do
    if p.position >= state.goal then
      return { winner = p.id }
    end
  end
  return nil
end
```

## セッションファイル

カレントディレクトリに自動生成される。`.gitignore` に追加推奨。

| ファイル | 内容 |
|---|---|
| `.bgt_config.json` | 使用中のgame.luaパス |
| `.bgt_state.json` | 現在のゲーム状態 |
| `.bgt_log.jsonl` | 全アクションのログ（1行1JSON） |

### ログの形式

```json
{
  "timestamp": "2026-03-17T11:34:43+09:00",
  "turn": "1-1",
  "player_id": 1,
  "action": {"type": "roll_die"},
  "state_before": { "..." },
  "state_after": { "..." }
}
```

`turn` は `ラウンド-ステップ` 形式。2人ゲームなら `1-1`, `1-2`, `2-1`, `2-2`, ... と進む。
