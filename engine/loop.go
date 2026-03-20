package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	configFile = ".bgt_config.json"
	stateFile  = ".bgt_state.json"
	logFile    = ".bgt_log.jsonl"
)

// Config はbgtの設定
type Config struct {
	GameLuaPath string `json:"game_lua_path"`
}

// Progression はゲーム進行の階層状態を管理する
type Progression struct {
	Round         int `json:"round"`
	Turn          int `json:"turn"`
	Step          int `json:"step"`
	CurrentPlayer int `json:"current_player"`
	NumPlayers    int `json:"num_players"`
}

// LogEntry はアクションログの1エントリ
type LogEntry struct {
	Timestamp   string                 `json:"timestamp"`
	Round       int                    `json:"round"`
	Turn        int                    `json:"turn"`
	Step        int                    `json:"step"`
	PlayerID    int                    `json:"player_id"`
	Action      map[string]interface{} `json:"action"`
	StateBefore map[string]interface{} `json:"state_before"`
	StateAfter  map[string]interface{} `json:"state_after"`
}

// Session はゲームセッションを管理する
type Session struct{}

// NewSession は新しいSessionを返す
func NewSession() *Session {
	return &Session{}
}

// Init はゲームロジックのパスを保存する
func (s *Session) Init(luaPath string) error {
	// Luaファイルの存在確認
	if _, err := os.Stat(luaPath); os.IsNotExist(err) {
		return fmt.Errorf("Luaファイルが見つかりません: %s", luaPath)
	}

	// 読み込みテスト
	eng, err := NewLuaEngine(luaPath)
	if err != nil {
		return err
	}
	eng.Close()

	cfg := Config{GameLuaPath: luaPath}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("設定ファイルの書き込みに失敗: %w", err)
	}

	fmt.Printf("ゲームを初期化しました: %s\n", luaPath)
	return nil
}

// Start はゲームを開始する
func (s *Session) Start(numPlayers int) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	eng, err := NewLuaEngine(cfg.GameLuaPath)
	if err != nil {
		return err
	}
	defer eng.Close()

	config := map[string]interface{}{
		"players": float64(numPlayers),
	}
	state, err := eng.Setup(config)
	if err != nil {
		return err
	}

	// _progression を注入
	prog := Progression{
		Round:         1,
		Turn:          1,
		Step:          1,
		CurrentPlayer: 1,
		NumPlayers:    numPlayers,
	}
	state["_progression"] = progressionToMap(prog)

	// Lua側のcurrent_player/turnは不要になるが、互換性のため残さない
	delete(state, "current_player")
	delete(state, "turn")

	// on_round_start を呼び出す（ラウンド1の開始）
	state, err = eng.OnRoundStart(state, 1)
	if err != nil {
		return err
	}
	// _progression を再注入（Lua側が消す可能性があるため）
	state["_progression"] = progressionToMap(prog)

	if err := s.saveState(state); err != nil {
		return err
	}

	// ログファイルをクリア
	os.Remove(logFile)

	fmt.Printf("ゲームを開始しました（%d人プレイヤー）\n", numPlayers)
	return nil
}

// Status は現在の状態を表示する
func (s *Session) Status() error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	state, err := s.loadState()
	if err != nil {
		return err
	}

	eng, err := NewLuaEngine(cfg.GameLuaPath)
	if err != nil {
		return err
	}
	defer eng.Close()

	prog := s.getProgression(state)

	// 終了判定
	terminal, err := eng.IsTerminal(state)
	if err != nil {
		return err
	}
	if terminal != nil {
		fmt.Println("ゲーム終了")
		termJSON, _ := json.MarshalIndent(terminal, "", "  ")
		fmt.Printf("結果: %s\n", termJSON)
		return nil
	}

	actions, err := eng.ValidActions(state, prog.CurrentPlayer)
	if err != nil {
		return err
	}

	// 可視状態をフィルタリング（_progressionを除外）
	visibleState, err := eng.VisibleState(state, prog.CurrentPlayer)
	if err != nil {
		return err
	}
	visibleState = s.excludeProgression(visibleState)

	fmt.Printf("現在のプレイヤー: Player %d\n", prog.CurrentPlayer)
	fmt.Printf("ラウンド: %d  ターン: %d  ステップ: %d\n", prog.Round, prog.Turn, prog.Step)
	stateJSON, _ := json.MarshalIndent(visibleState, "", "  ")
	fmt.Printf("state: %s\n", stateJSON)
	actionsJSON, _ := json.Marshal(actions)
	fmt.Printf("取れるアクション: %s\n", actionsJSON)
	return nil
}

// Do はアクションを実行する
func (s *Session) Do(actionType string, args map[string]string) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	state, err := s.loadState()
	if err != nil {
		return err
	}

	eng, err := NewLuaEngine(cfg.GameLuaPath)
	if err != nil {
		return err
	}
	defer eng.Close()

	prog := s.getProgression(state)

	actions, err := eng.ValidActions(state, prog.CurrentPlayer)
	if err != nil {
		return err
	}

	// アクションの検証
	valid := false
	for _, a := range actions {
		if a["type"] == actionType {
			valid = true
			break
		}
	}
	if !valid {
		validTypes := make([]string, 0, len(actions))
		for _, a := range actions {
			if t, ok := a["type"].(string); ok {
				validTypes = append(validTypes, t)
			}
		}
		return fmt.Errorf("無効なアクション: %s（有効: %s）", actionType, strings.Join(validTypes, ", "))
	}

	// アクションを構築
	action := map[string]interface{}{
		"type": actionType,
	}
	for k, v := range args {
		action[k] = v
	}

	// ステップを実行
	newState, terminated, err := s.executeStep(eng, state, action, prog)
	if err != nil {
		return err
	}

	newProg := s.getProgression(newState)
	fmt.Printf("Player %d が %s を実行しました\n", prog.CurrentPlayer, actionType)

	if terminated != nil {
		fmt.Println("ゲーム終了！")
		termJSON, _ := json.MarshalIndent(terminated, "", "  ")
		fmt.Printf("結果: %s\n", termJSON)
	} else {
		fmt.Printf("次: Player %d（R%d T%d-%d）\n", newProg.CurrentPlayer, newProg.Round, newProg.Turn, newProg.Step)
	}

	return nil
}

// AI は現在のプレイヤーをClaudeに判断させる
func (s *Session) AI() error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	state, err := s.loadState()
	if err != nil {
		return err
	}

	eng, err := NewLuaEngine(cfg.GameLuaPath)
	if err != nil {
		return err
	}
	defer eng.Close()

	prog := s.getProgression(state)

	actions, err := eng.ValidActions(state, prog.CurrentPlayer)
	if err != nil {
		return err
	}

	claude, err := NewClaudePlayer()
	if err != nil {
		return err
	}

	visibleState, err := eng.VisibleState(state, prog.CurrentPlayer)
	if err != nil {
		return err
	}
	visibleState = s.excludeProgression(visibleState)

	description, err := eng.Describe(state, prog.CurrentPlayer)
	if err != nil {
		return err
	}

	chosen, err := claude.ChooseAction(visibleState, actions, prog.CurrentPlayer, description)
	if err != nil {
		return err
	}

	fmt.Printf("AIが選択: %v\n", chosen)

	newState, terminated, err := s.executeStep(eng, state, chosen, prog)
	if err != nil {
		return err
	}

	if terminated != nil {
		fmt.Println("ゲーム終了！")
		termJSON, _ := json.MarshalIndent(terminated, "", "  ")
		fmt.Printf("結果: %s\n", termJSON)
	}

	_ = newState
	return nil
}

// Auto は終了まで全プレイヤーをAIで自動進行する
func (s *Session) Auto() error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}

	state, err := s.loadState()
	if err != nil {
		return err
	}

	eng, err := NewLuaEngine(cfg.GameLuaPath)
	if err != nil {
		return err
	}
	defer eng.Close()

	claude, err := NewClaudePlayer()
	if err != nil {
		return err
	}

	prevRound := 0

	for {
		// 終了判定
		terminal, err := eng.IsTerminal(state)
		if err != nil {
			return err
		}
		if terminal != nil {
			fmt.Println("\nゲーム終了！")
			termJSON, _ := json.MarshalIndent(terminal, "", "  ")
			fmt.Printf("結果: %s\n", termJSON)
			return nil
		}

		prog := s.getProgression(state)

		// ラウンド変更の表示
		if prevRound != 0 && prog.Round != prevRound {
			fmt.Printf("--- ラウンド %d 終了 ---\n", prevRound)
			fmt.Printf("--- ラウンド %d 開始 ---\n", prog.Round)
		} else if prevRound == 0 {
			// 初回
			prevRound = prog.Round
		}

		actions, err := eng.ValidActions(state, prog.CurrentPlayer)
		if err != nil {
			return err
		}

		visibleState, err := eng.VisibleState(state, prog.CurrentPlayer)
		if err != nil {
			return err
		}
		visibleState = s.excludeProgression(visibleState)

		description, err := eng.Describe(state, prog.CurrentPlayer)
		if err != nil {
			return err
		}

		chosen, err := claude.ChooseAction(visibleState, actions, prog.CurrentPlayer, description)
		if err != nil {
			return err
		}

		chosenJSON, _ := json.Marshal(chosen)
		fmt.Printf("[R%d T%d-%d] Player %d: %s\n", prog.Round, prog.Turn, prog.Step, prog.CurrentPlayer, chosenJSON)

		prevRound = prog.Round

		newState, terminated, err := s.executeStep(eng, state, chosen, prog)
		if err != nil {
			return err
		}

		if terminated != nil {
			fmt.Println("\nゲーム終了！")
			termJSON, _ := json.MarshalIndent(terminated, "", "  ")
			fmt.Printf("結果: %s\n", termJSON)
			return nil
		}

		state = newState
	}
}

func (s *Session) loadConfig() (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルが見つかりません。先に bgt init を実行してください")
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルの読み込みに失敗: %w", err)
	}
	return &cfg, nil
}

func (s *Session) loadState() (map[string]interface{}, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("状態ファイルが見つかりません。先に bgt start を実行してください")
	}

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var state map[string]interface{}
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("状態ファイルの読み込みに失敗: %w", err)
	}
	return state, nil
}

func (s *Session) saveState(state map[string]interface{}) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("状態のJSON変換に失敗: %w", err)
	}
	return os.WriteFile(stateFile, data, 0644)
}

func (s *Session) appendLog(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
	f.Write([]byte("\n"))
}

// getProgression はstateから_progressionを取得する
func (s *Session) getProgression(state map[string]interface{}) Progression {
	prog := Progression{Round: 1, Turn: 1, Step: 1, CurrentPlayer: 1, NumPlayers: 1}
	raw, ok := state["_progression"]
	if !ok {
		return prog
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return prog
	}
	if v := toInt(m["round"]); v > 0 {
		prog.Round = v
	}
	if v := toInt(m["turn"]); v > 0 {
		prog.Turn = v
	}
	if v := toInt(m["step"]); v > 0 {
		prog.Step = v
	}
	if v := toInt(m["current_player"]); v > 0 {
		prog.CurrentPlayer = v
	}
	if v := toInt(m["num_players"]); v > 0 {
		prog.NumPlayers = v
	}
	return prog
}

// progressionToMap はProgressionをmap[string]interface{}に変換する
func progressionToMap(p Progression) map[string]interface{} {
	return map[string]interface{}{
		"round":          float64(p.Round),
		"turn":           float64(p.Turn),
		"step":           float64(p.Step),
		"current_player": float64(p.CurrentPlayer),
		"num_players":    float64(p.NumPlayers),
	}
}

// excludeProgression はstateのコピーから_progressionを除外して返す
func (s *Session) excludeProgression(state map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(state))
	for k, v := range state {
		if k == "_progression" {
			continue
		}
		result[k] = v
	}
	return result
}

// executeStep はアクションを実行し、progressionを更新し、ラウンド終了判定を行う
func (s *Session) executeStep(eng *LuaEngine, state map[string]interface{}, action map[string]interface{}, prog Progression) (map[string]interface{}, map[string]interface{}, error) {
	// apply_action を呼び出す
	newState, err := eng.ApplyAction(state, action, prog.CurrentPlayer)
	if err != nil {
		return nil, nil, err
	}

	// Lua側が返したcurrent_player/turnを_progressionの値で上書き
	delete(newState, "current_player")
	delete(newState, "turn")

	// ログ記録
	s.appendLog(LogEntry{
		Timestamp:   time.Now().Format(time.RFC3339),
		Round:       prog.Round,
		Turn:        prog.Turn,
		Step:        prog.Step,
		PlayerID:    prog.CurrentPlayer,
		Action:      action,
		StateBefore: state,
		StateAfter:  newState,
	})

	// progression を進める
	newProg := prog
	newProg.Step++
	newProg.CurrentPlayer = (prog.CurrentPlayer % prog.NumPlayers) + 1
	if newProg.Step > newProg.NumPlayers {
		newProg.Turn++
		newProg.Step = 1
	}

	// _progression を更新
	newState["_progression"] = progressionToMap(newProg)

	// is_round_over チェック
	roundOver, err := eng.IsRoundOver(newState)
	if err != nil {
		return nil, nil, err
	}

	if roundOver {
		// on_round_end
		newState, err = eng.OnRoundEnd(newState)
		if err != nil {
			return nil, nil, err
		}
		// _progression を再注入（Lua側が消す可能性があるため）
		newState["_progression"] = progressionToMap(newProg)

		// is_terminal チェック（ラウンド終了時）
		terminal, err := eng.IsTerminal(newState)
		if err != nil {
			return nil, nil, err
		}
		if terminal != nil {
			if err := s.saveState(newState); err != nil {
				return nil, nil, err
			}
			return newState, terminal, nil
		}

		// 次のラウンドへ
		newProg.Round++
		newProg.Turn = 1
		newProg.Step = 1
		newProg.CurrentPlayer = 1
		newState["_progression"] = progressionToMap(newProg)

		// on_round_start
		newState, err = eng.OnRoundStart(newState, newProg.Round)
		if err != nil {
			return nil, nil, err
		}
		// _progression を再注入
		newState["_progression"] = progressionToMap(newProg)
	}

	if err := s.saveState(newState); err != nil {
		return nil, nil, err
	}

	return newState, nil, nil
}

// toInt は各種数値型をintに変換する
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case int:
		return n
	}
	return 0
}
