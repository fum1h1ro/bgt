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

返すstateには **`players`**（プレイヤーの配列）を含めること。`current_player` や `turn` はGoが `_progression` として自動管理するため、返す必要はない。

```lua
function setup(config)
  local players = {}
  for i = 1, config.players do
    players[i] = { id = i, score = 0 }
  end
  return {
    players = players,
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

プレイヤー交代やターン管理はGoが自動で行うため、`current_player` や `turn` の更新は不要。

```lua
function apply_action(state, action, player_id)
  local new_state = deep_copy(state)

  if action.type == "draw_card" then
    -- ゲーム固有の処理
  end

  return new_state
end
```

#### `is_terminal(state) → result or nil`

ゲームが終了していれば結果テーブルを、継続中なら `nil` を返す。ラウンド終了時にのみ呼び出される。

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

### オプション関数

#### `is_round_over(state) → true or nil`

ラウンドが終了したかを判定する。毎ステップ後に呼び出される。未定義の場合は「1ターン完了（全員1回行動）= ラウンド終了」。

```lua
function is_round_over(state)
  -- 例: 特定の条件でラウンドを終了させる
  if state.round_should_end then
    return true
  end
  return nil
end
```

#### `on_round_start(state, round_number) → state`

ラウンド開始時に呼び出される。カードを配り直すなどの処理に使う。未定義の場合はstateそのまま。

```lua
function on_round_start(state, round_number)
  local new_state = deep_copy(state)
  -- 手札を配り直すなど
  return new_state
end
```

#### `on_round_end(state) → state`

ラウンド終了時に呼び出される（`is_terminal` の前）。スコア集計などに使う。未定義の場合はstateそのまま。

```lua
function on_round_end(state)
  local new_state = deep_copy(state)
  -- スコア集計など
  return new_state
end
```

#### `visible_state(state, player_id) → filtered_state`

指定プレイヤーに見える状態を返す。`bgt status` の表示とAIプレイヤーへの入力の両方でフィルタリングに使われる。未定義の場合は全stateがそのまま使われる。

```lua
function visible_state(state, player_id)
  local s = deep_copy(state)
  s.deck = nil  -- 山札は誰にも見えない
  for _, p in ipairs(s.players) do
    if p.id ~= player_id then
      p.hand = nil  -- 他プレイヤーの手札を隠す
    end
  end
  return s
end
```

#### `describe(state, player_id) → string`

ゲームのルールと現在の状況を自然言語で返す。AIプレイヤーのプロンプトに挿入され、判断精度を向上させる。未定義の場合はルール説明なしで動作する。

```lua
function describe(state, player_id)
  return "【すごろくゲーム】\n"
    .. "ルール: サイコロ(1-6)を振って進み、マス20に最初に到達したプレイヤーが勝利。\n"
    .. "あなたはPlayer " .. player_id .. "です。"
end
```

### stateの規約

| フィールド | 型 | 説明 |
|---|---|---|
| `players` | table(配列) | プレイヤー情報の配列。**必須** |
| `_progression` | table | Goが自動管理（round, turn, step, current_player, num_players）。**Lua側では触らない** |
| その他 | 任意 | ゲーム固有の状態を自由に追加 |

ゲーム進行は Session > Round > Turn > Step の4層で管理される。`_progression` は `bgt status` やAIプレイヤーへの表示時には自動的に除外される。

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
    goal = 20,
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
  "round": 1,
  "turn": 1,
  "step": 1,
  "player_id": 1,
  "action": {"type": "roll_die"},
  "state_before": { "..." },
  "state_after": { "..." }
}
```

`round`, `turn`, `step` はそれぞれ整数値。3人ゲームなら step が 1→2→3 と進み、全員行動するとturnが進み、ラウンド終了条件を満たすとroundが進む。
