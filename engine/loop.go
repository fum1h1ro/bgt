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

// LogEntry はアクションログの1エントリ
type LogEntry struct {
	Timestamp     string                 `json:"timestamp"`
	Turn          string                 `json:"turn"`
	PlayerID      int                    `json:"player_id"`
	Action        map[string]interface{} `json:"action"`
	StateBefore   map[string]interface{} `json:"state_before"`
	StateAfter    map[string]interface{} `json:"state_after"`
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

	currentPlayer := s.getCurrentPlayer(state)

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

	actions, err := eng.ValidActions(state, currentPlayer)
	if err != nil {
		return err
	}

	// 可視状態をフィルタリング
	visibleState, err := eng.VisibleState(state, currentPlayer)
	if err != nil {
		return err
	}

	fmt.Printf("現在のプレイヤー: Player %d\n", currentPlayer)
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

	currentPlayer := s.getCurrentPlayer(state)

	actions, err := eng.ValidActions(state, currentPlayer)
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

	// アクションを適用
	newState, err := eng.ApplyAction(state, action, currentPlayer)
	if err != nil {
		return err
	}

	if err := s.saveState(newState); err != nil {
		return err
	}

	s.appendLog(LogEntry{
		Timestamp:   time.Now().Format(time.RFC3339),
		Turn:        s.calcTurn(state),
		PlayerID:    currentPlayer,
		Action:      action,
		StateBefore: state,
		StateAfter:  newState,
	})

	fmt.Printf("Player %d が %s を実行しました\n", currentPlayer, actionType)

	// 終了判定
	terminal, err := eng.IsTerminal(newState)
	if err != nil {
		return err
	}
	if terminal != nil {
		fmt.Println("ゲーム終了！")
		termJSON, _ := json.MarshalIndent(terminal, "", "  ")
		fmt.Printf("結果: %s\n", termJSON)
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

	currentPlayer := s.getCurrentPlayer(state)

	actions, err := eng.ValidActions(state, currentPlayer)
	if err != nil {
		return err
	}

	claude, err := NewClaudePlayer()
	if err != nil {
		return err
	}

	visibleState, err := eng.VisibleState(state, currentPlayer)
	if err != nil {
		return err
	}

	description, err := eng.Describe(state, currentPlayer)
	if err != nil {
		return err
	}

	chosen, err := claude.ChooseAction(visibleState, actions, currentPlayer, description)
	if err != nil {
		return err
	}

	fmt.Printf("AIが選択: %v\n", chosen)

	newState, err := eng.ApplyAction(state, chosen, currentPlayer)
	if err != nil {
		return err
	}

	if err := s.saveState(newState); err != nil {
		return err
	}

	s.appendLog(LogEntry{
		Timestamp:   time.Now().Format(time.RFC3339),
		Turn:        s.calcTurn(state),
		PlayerID:    currentPlayer,
		Action:      chosen,
		StateBefore: state,
		StateAfter:  newState,
	})

	// 終了判定
	terminal, err := eng.IsTerminal(newState)
	if err != nil {
		return err
	}
	if terminal != nil {
		fmt.Println("ゲーム終了！")
		termJSON, _ := json.MarshalIndent(terminal, "", "  ")
		fmt.Printf("結果: %s\n", termJSON)
	}

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

		currentPlayer := s.getCurrentPlayer(state)

		actions, err := eng.ValidActions(state, currentPlayer)
		if err != nil {
			return err
		}

		visibleState, err := eng.VisibleState(state, currentPlayer)
		if err != nil {
			return err
		}

		description, err := eng.Describe(state, currentPlayer)
		if err != nil {
			return err
		}

		chosen, err := claude.ChooseAction(visibleState, actions, currentPlayer, description)
		if err != nil {
			return err
		}

		turnStr := s.calcTurn(state)
		fmt.Printf("[ターン%s] Player %d: %v\n", turnStr, currentPlayer, chosen)

		newState, err := eng.ApplyAction(state, chosen, currentPlayer)
		if err != nil {
			return err
		}

		if err := s.saveState(newState); err != nil {
			return err
		}

		s.appendLog(LogEntry{
			Timestamp:   time.Now().Format(time.RFC3339),
			PlayerID:    currentPlayer,
			Action:      chosen,
			StateBefore: state,
			StateAfter:  newState,
		})

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

func (s *Session) getCurrentPlayer(state map[string]interface{}) int {
	if cp, ok := state["current_player"]; ok {
		switch v := cp.(type) {
		case float64:
			return int(v)
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		}
	}
	return 1
}

// getNumPlayers はstateからプレイヤー数を取得する
func (s *Session) getNumPlayers(state map[string]interface{}) int {
	if players, ok := state["players"]; ok {
		if arr, ok := players.([]interface{}); ok {
			return len(arr)
		}
	}
	return 1
}

// calcTurn はstateのturn（1始まりの連番）からx-y形式のターン文字列を返す
// x: ラウンド（全員が1回ずつプレイで1ラウンド）、y: ラウンド内のステップ
func (s *Session) calcTurn(state map[string]interface{}) string {
	turn := 1
	if t, ok := state["turn"]; ok {
		switch v := t.(type) {
		case float64:
			turn = int(v)
		case json.Number:
			n, _ := v.Int64()
			turn = int(n)
		}
	}
	numPlayers := s.getNumPlayers(state)
	round := (turn-1)/numPlayers + 1
	step := (turn-1)%numPlayers + 1
	return fmt.Sprintf("%d-%d", round, step)
}
