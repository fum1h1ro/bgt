package engine

import (
	"math/rand"
	"time"
)

var globalRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// SetSeed はグローバル乱数生成器のシードを設定する
func SetSeed(seed int64) {
	globalRand = rand.New(rand.NewSource(seed))
}

// NewSeed は時刻ベースの新しいシードを生成する
func NewSeed() int64 {
	return time.Now().UnixNano()
}
