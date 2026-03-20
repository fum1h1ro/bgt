-- kaiteitan: シンプルなすごろくゲーム
-- ゴール（マス20）に最初にたどり着いたプレイヤーが勝利

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

function describe(state, player_id)
  local positions = {}
  for _, p in ipairs(state.players) do
    positions[#positions + 1] = "Player " .. p.id .. ": マス" .. p.position
  end
  return "【すごろくゲーム】\n"
    .. "ルール: サイコロ(1-6)を振って進み、マス" .. state.goal .. "に最初に到達したプレイヤーが勝利。\n"
    .. "現在位置: " .. table.concat(positions, ", ") .. "\n"
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
