package database

import (
	"time"

	"gorm.io/gorm"
)

// Model is the base struct embedded by every entity. Timestamps are stored as
// Unix milliseconds in UTC.
type Model struct {
	ID        string `json:"id" gorm:"column:id;primaryKey;type:varchar(40)"`
	CreatedAt int64  `json:"created_at" gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt int64  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime:false"`
}

// GetID returns the primary key.
func (m *Model) GetID() string { return m.ID }

// Touch sets CreatedAt if unset and always advances UpdatedAt.
func (m *Model) Touch(now time.Time) {
	ms := now.UTC().UnixMilli()
	if m.CreatedAt == 0 {
		m.CreatedAt = ms
	}
	m.UpdatedAt = ms
}

// BeforeCreate fills timestamps left unset by the caller. IDs are assigned by
// the entity's own hook, which knows its prefix.
func (m *Model) BeforeCreate(*gorm.DB) error {
	m.Touch(time.Now())
	return nil
}

// BeforeUpdate advances UpdatedAt.
func (m *Model) BeforeUpdate(*gorm.DB) error {
	m.UpdatedAt = time.Now().UTC().UnixMilli()
	return nil
}

// SoftDeletableModel adds a soft-delete marker. Queries must exclude rows with
// a non-nil DeletedAt.
type SoftDeletableModel struct {
	Model
	DeletedAt *int64 `json:"deleted_at" gorm:"column:deleted_at;index"`
}

// IsDeleted reports whether the row is soft-deleted.
func (m *SoftDeletableModel) IsDeleted() bool { return m.DeletedAt != nil }

// MarkDeleted sets the soft-delete marker.
func (m *SoftDeletableModel) MarkDeleted(now time.Time) {
	ms := now.UTC().UnixMilli()
	m.DeletedAt = &ms
	m.UpdatedAt = ms
}

// Entity is implemented by every persisted model.
type Entity interface {
	TableName() string
	EntityName() string
	GetID() string
}
