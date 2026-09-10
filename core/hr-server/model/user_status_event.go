package model

import "time"

// UserStatusEvent records explicit administrator actions, never per-request activity.
type UserStatusEvent struct {
	ID             uint64    `json:"id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"created_at"`
	Username       string    `json:"username" gorm:"size:255;index"`
	Actor          string    `json:"actor" gorm:"size:255"`
	PreviousStatus string    `json:"previous_status" gorm:"size:50"`
	Status         string    `json:"status" gorm:"size:50"`
	Reason         string    `json:"reason" gorm:"size:500"`
}

func (UserStatusEvent) TableName() string { return "user_status_event" }
