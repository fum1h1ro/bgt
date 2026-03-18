package engine

import (
	"encoding/json"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// LuaEngine はLuaゲームロジックとのブリッジ
type LuaEngine struct {
	L *lua.LState
}

// NewLuaEngine はLuaファイルを読み込みエンジンを初期化する
func NewLuaEngine(luaFilePath string) (*LuaEngine, error) {
	L := lua.NewState()

	// bgtグローバルテーブルを登録（ランダム処理など）
	registerBgtGlobals(L)

	// deep_copyをグローバル関数として登録
	registerDeepCopy(L)

	if err := L.DoFile(luaFilePath); err != nil {
		L.Close()
		return nil, fmt.Errorf("Luaファイルの読み込みに失敗: %w", err)
	}

	return &LuaEngine{L: L}, nil
}

// Close はLStateを閉じる
func (e *LuaEngine) Close() {
	e.L.Close()
}

// Setup はLuaのsetup(config)を呼び出す
func (e *LuaEngine) Setup(config map[string]interface{}) (map[string]interface{}, error) {
	fn := e.L.GetGlobal("setup")
	if fn == lua.LNil {
		return nil, fmt.Errorf("Lua関数 setup が定義されていません")
	}

	configLua := goToLua(e.L, config)
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, configLua); err != nil {
		return nil, fmt.Errorf("setup() の呼び出しに失敗: %w", err)
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	result, ok := luaToGo(ret).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("setup() がテーブルを返しませんでした")
	}
	return result, nil
}

// ValidActions はLuaのvalid_actions(state, player_id)を呼び出す
func (e *LuaEngine) ValidActions(state map[string]interface{}, playerID int) ([]map[string]interface{}, error) {
	fn := e.L.GetGlobal("valid_actions")
	if fn == lua.LNil {
		return nil, fmt.Errorf("Lua関数 valid_actions が定義されていません")
	}

	stateLua := goToLua(e.L, state)
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, stateLua, lua.LNumber(playerID)); err != nil {
		return nil, fmt.Errorf("valid_actions() の呼び出しに失敗: %w", err)
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	rawActions, ok := luaToGo(ret).([]interface{})
	if !ok {
		return nil, fmt.Errorf("valid_actions() が配列を返しませんでした")
	}

	actions := make([]map[string]interface{}, 0, len(rawActions))
	for _, a := range rawActions {
		action, ok := a.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("valid_actions() の要素がテーブルではありません")
		}
		actions = append(actions, action)
	}
	return actions, nil
}

// ApplyAction はLuaのapply_action(state, action, player_id)を呼び出す
func (e *LuaEngine) ApplyAction(state map[string]interface{}, action map[string]interface{}, playerID int) (map[string]interface{}, error) {
	fn := e.L.GetGlobal("apply_action")
	if fn == lua.LNil {
		return nil, fmt.Errorf("Lua関数 apply_action が定義されていません")
	}

	stateLua := goToLua(e.L, state)
	actionLua := goToLua(e.L, action)
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, stateLua, actionLua, lua.LNumber(playerID)); err != nil {
		return nil, fmt.Errorf("apply_action() の呼び出しに失敗: %w", err)
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	result, ok := luaToGo(ret).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("apply_action() がテーブルを返しませんでした")
	}
	return result, nil
}

// VisibleState はLuaのvisible_state(state, player_id)を呼び出す
// 未定義の場合はstateをそのまま返す
func (e *LuaEngine) VisibleState(state map[string]interface{}, playerID int) (map[string]interface{}, error) {
	fn := e.L.GetGlobal("visible_state")
	if fn == lua.LNil {
		return state, nil
	}

	stateLua := goToLua(e.L, state)
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, stateLua, lua.LNumber(playerID)); err != nil {
		return nil, fmt.Errorf("visible_state() の呼び出しに失敗: %w", err)
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	result, ok := luaToGo(ret).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("visible_state() がテーブルを返しませんでした")
	}
	return result, nil
}

// Describe はLuaのdescribe(state, player_id)を呼び出す
// 未定義の場合は空文字を返す
func (e *LuaEngine) Describe(state map[string]interface{}, playerID int) (string, error) {
	fn := e.L.GetGlobal("describe")
	if fn == lua.LNil {
		return "", nil
	}

	stateLua := goToLua(e.L, state)
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, stateLua, lua.LNumber(playerID)); err != nil {
		return "", fmt.Errorf("describe() の呼び出しに失敗: %w", err)
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	if ret == lua.LNil {
		return "", nil
	}

	if s, ok := ret.(lua.LString); ok {
		return string(s), nil
	}
	return "", fmt.Errorf("describe() が文字列を返しませんでした")
}

// IsTerminal はLuaのis_terminal(state)を呼び出す
func (e *LuaEngine) IsTerminal(state map[string]interface{}) (map[string]interface{}, error) {
	fn := e.L.GetGlobal("is_terminal")
	if fn == lua.LNil {
		return nil, fmt.Errorf("Lua関数 is_terminal が定義されていません")
	}

	stateLua := goToLua(e.L, state)
	if err := e.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, stateLua); err != nil {
		return nil, fmt.Errorf("is_terminal() の呼び出しに失敗: %w", err)
	}

	ret := e.L.Get(-1)
	e.L.Pop(1)

	if ret == lua.LNil {
		return nil, nil
	}

	result, ok := luaToGo(ret).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("is_terminal() がテーブルまたはnilを返しませんでした")
	}
	return result, nil
}

// goToLua はGoの値をLuaの値に変換する
func goToLua(L *lua.LState, value interface{}) lua.LValue {
	if value == nil {
		return lua.LNil
	}
	switch v := value.(type) {
	case map[string]interface{}:
		tbl := L.NewTable()
		for key, val := range v {
			tbl.RawSetString(key, goToLua(L, val))
		}
		return tbl
	case []interface{}:
		tbl := L.NewTable()
		for i, val := range v {
			tbl.RawSetInt(i+1, goToLua(L, val))
		}
		return tbl
	case string:
		return lua.LString(v)
	case float64:
		return lua.LNumber(v)
	case json.Number:
		f, _ := v.Float64()
		return lua.LNumber(f)
	case int:
		return lua.LNumber(v)
	case bool:
		return lua.LBool(v)
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// luaToGo はLuaの値をGoの値に変換する
func luaToGo(value lua.LValue) interface{} {
	switch v := value.(type) {
	case *lua.LTable:
		return luaTableToGo(v)
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case *lua.LNilType:
		return nil
	case lua.LString:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// luaTableToGo はLTableをGoのmapまたはsliceに変換する
func luaTableToGo(tbl *lua.LTable) interface{} {
	maxN := tbl.MaxN()

	// 文字列キーがあるかチェック
	hasStringKeys := false
	tbl.ForEach(func(key, _ lua.LValue) {
		if _, ok := key.(lua.LString); ok {
			hasStringKeys = true
		}
	})

	// 連番キーのみなら配列として扱う
	if maxN > 0 && !hasStringKeys {
		arr := make([]interface{}, 0, maxN)
		for i := 1; i <= maxN; i++ {
			arr = append(arr, luaToGo(tbl.RawGetInt(i)))
		}
		return arr
	}

	// それ以外はmapとして扱う
	result := make(map[string]interface{})
	tbl.ForEach(func(key, val lua.LValue) {
		switch k := key.(type) {
		case lua.LString:
			result[string(k)] = luaToGo(val)
		case lua.LNumber:
			result[fmt.Sprintf("%v", float64(k))] = luaToGo(val)
		}
	})
	return result
}

// registerBgtGlobals はbgtグローバルテーブルをLuaに登録する
func registerBgtGlobals(L *lua.LState) {
	bgt := L.NewTable()

	// bgt.roll(sides) -- 1からsidesまでのランダム整数
	L.SetField(bgt, "roll", L.NewFunction(func(L *lua.LState) int {
		sides := L.CheckInt(1)
		result := globalRand.Intn(sides) + 1
		L.Push(lua.LNumber(result))
		return 1
	}))

	// bgt.random(min, max) -- minからmaxまでのランダム整数
	L.SetField(bgt, "random", L.NewFunction(func(L *lua.LState) int {
		min := L.CheckInt(1)
		max := L.CheckInt(2)
		result := globalRand.Intn(max-min+1) + min
		L.Push(lua.LNumber(result))
		return 1
	}))

	L.SetGlobal("bgt", bgt)
}

// registerDeepCopy はdeep_copyグローバル関数をLuaに登録する
func registerDeepCopy(L *lua.LState) {
	L.SetGlobal("deep_copy", L.NewFunction(func(L *lua.LState) int {
		orig := L.CheckTable(1)
		copy := deepCopyTable(L, orig)
		L.Push(copy)
		return 1
	}))
}

func deepCopyTable(L *lua.LState, orig *lua.LTable) *lua.LTable {
	copy := L.NewTable()
	orig.ForEach(func(key, val lua.LValue) {
		if tbl, ok := val.(*lua.LTable); ok {
			copy.RawSet(key, deepCopyTable(L, tbl))
		} else {
			copy.RawSet(key, val)
		}
	})
	return copy
}
