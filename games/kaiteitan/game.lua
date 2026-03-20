-- kaiteitan: シンプルなすごろくゲーム
-- ゴール（マス20）に最初にたどり着いたプレイヤーが勝利

function setup(config)
  local players = {}
  for i = 1, config.players do
    players[i] = { id = i, position = 0, charged = false }
  end
  return {
    players = players,
    goal = 20,
    last_roll = nil,
    last_player = nil,
  }
end

function valid_actions(state, player_id)
  if state.players[player_id].charged then
    return {
      { type = "roll_die" }
    }
  else
    return {
      { type = "roll_die" },
      { type = "charge" },
    }
  end
end

function apply_action(state, action, player_id)
  local new_state = deep_copy(state)
  if action.type == "roll_die" then
    local roll = bgt.roll(6)
    if state.players[player_id].charged then
      roll = roll * 2
    end
    local p = new_state.players[player_id]
    p.position = p.position + roll
    p.charged = false
    if p.position > new_state.goal then
      p.position = new_state.goal
    end
    new_state.last_roll = roll
    new_state.last_player = player_id
  elseif action.type == "charge" then
    local p = new_state.players[player_id]
    p.charged = true
  end
  return new_state
end

function describe(state, player_id)
  local positions = {}
  for _, p in ipairs(state.players) do
    positions[#positions + 1] = "Player " .. p.id .. ": マス" .. p.position
  end

  local last_action = ""
  if state.last_roll then
    last_action = "直前: Player " .. state.last_player .. " がサイコロで " .. state.last_roll .. " を出した\n"
  end

  return "【すごろくゲーム】\n"
    .. "ルール: サイコロ(1-6)を振って進み、マス" .. state.goal .. "に最初に到達したプレイヤーが勝利。\n"
    .. "charge すると、次の手番の時サイコロの目が2倍になる。\n"
    .. "現在位置: " .. table.concat(positions, ", ") .. "\n"
    .. last_action
    .. "あなたはPlayer " .. player_id .. "です。"
end

function is_terminal(state)
  for _, p in ipairs(state.players) do
    if p.position >= state.goal then
      return { winner = p.id }
    end
  end
  return nil
end
