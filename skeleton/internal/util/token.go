package util

import (
	"fmt"
	"time"
)

func NewTOken() string {
	return fmt.Sprintf("u%d", time.Now().UnixNano())
}