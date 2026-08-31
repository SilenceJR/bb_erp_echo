package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const singletonIdentityID uint = 1

// IdentityMetadata 是创建或刷新服务身份公开元数据时使用的输入。
// InstanceID 不在此结构中，确保服务名或版本变化不会意外更换实例身份。
type IdentityMetadata struct {
	Product           string
	DiscoveryProtocol int
	ServerName        string
	ServerVersion     string
}

// Identity 是 SQLite 中保存的服务单例身份。
//
// ID 固定为 1。InstanceID 只在首次创建时生成，服务重启或版本更新只会
// 刷新公开的名称和版本，保证客户端能够识别同一个内网服务实例。
type Identity struct {
	ID                uint   `json:"-" gorm:"primaryKey"`
	InstanceID        string `json:"instance_id" gorm:"size:128;not null;uniqueIndex"`
	Product           string `json:"product" gorm:"size:32;not null"`
	DiscoveryProtocol int    `json:"discovery_protocol" gorm:"not null"`
	ServerName        string `json:"server_name" gorm:"size:120;not null"`
	ServerVersion     string `json:"server_version" gorm:"size:64;not null"`
}

// TableName 指定身份表名，避免与其他 Identity 类型发生默认表名碰撞。
func (Identity) TableName() string { return "discovery_identities" }

// LoadOrCreate 在 SQLite 中加载固定单例身份；不存在时创建一次随机 UUID。
//
// 该操作在事务中完成。已有身份的 InstanceID 永不覆盖；如果数据库中已有
// 损坏的空身份，则直接阻断启动，避免客户端把一次服务误认为新的实例。
func LoadOrCreate(db *gorm.DB, metadata IdentityMetadata) (*Identity, error) {
	if db == nil {
		return nil, errors.New("load discovery identity: database is nil")
	}
	if err := validateMetadata(metadata); err != nil {
		return nil, err
	}

	var identity Identity
	err := db.Transaction(func(tx *gorm.DB) error {
		result := tx.First(&identity, singletonIdentityID)
		switch {
		case result.Error == nil:
			if err := validatePersistedIdentity(identity); err != nil {
				return err
			}
		case errors.Is(result.Error, gorm.ErrRecordNotFound):
			instanceID, err := newInstanceID()
			if err != nil {
				return err
			}
			identity = Identity{
				ID:                singletonIdentityID,
				InstanceID:        instanceID,
				Product:           metadata.Product,
				DiscoveryProtocol: metadata.DiscoveryProtocol,
				ServerName:        metadata.ServerName,
				ServerVersion:     metadata.ServerVersion,
			}
			if err := tx.Create(&identity).Error; err != nil {
				return fmt.Errorf("create discovery identity: %w", err)
			}
			return nil
		default:
			return fmt.Errorf("load discovery identity: %w", result.Error)
		}

		updates := map[string]any{
			"product":            metadata.Product,
			"discovery_protocol": metadata.DiscoveryProtocol,
			"server_name":        metadata.ServerName,
			"server_version":     metadata.ServerVersion,
		}
		if err := tx.Model(&identity).Updates(updates).Error; err != nil {
			return fmt.Errorf("refresh discovery identity metadata: %w", err)
		}
		identity.Product = metadata.Product
		identity.DiscoveryProtocol = metadata.DiscoveryProtocol
		identity.ServerName = metadata.ServerName
		identity.ServerVersion = metadata.ServerVersion
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

// Response 返回匿名 HTTP 接口使用的最小身份，不暴露数据库主键。
func (i Identity) Response() IdentityResponse {
	return IdentityResponse{
		Product:           i.Product,
		DiscoveryProtocol: i.DiscoveryProtocol,
		InstanceID:        i.InstanceID,
		ServerName:        i.ServerName,
		ServerVersion:     i.ServerVersion,
	}
}

// Announcement 返回给指定 nonce 的 UDP 发现响应。
func (i Identity) Announcement(nonce string, httpPort int) Announcement {
	return Announcement{
		Kind:       "announce",
		Protocol:   ProtocolVersion,
		Nonce:      nonce,
		Product:    Product,
		InstanceID: i.InstanceID,
		ServerName: i.ServerName,
		HTTPPort:   httpPort,
	}
}

func validateMetadata(metadata IdentityMetadata) error {
	if metadata.Product != Product {
		return fmt.Errorf("discovery identity product must be %q", Product)
	}
	if metadata.DiscoveryProtocol != ProtocolVersion {
		return fmt.Errorf("discovery identity protocol must be %d", ProtocolVersion)
	}
	if err := validateText("server_name", metadata.ServerName, 120); err != nil {
		return err
	}
	if err := validateText("server_version", metadata.ServerVersion, 64); err != nil {
		return err
	}
	return nil
}

func validatePersistedIdentity(identity Identity) error {
	if identity.ID != singletonIdentityID {
		return errors.New("discovery identity singleton row has an invalid id")
	}
	if err := validateInstanceID(identity.InstanceID); err != nil {
		return fmt.Errorf("stored discovery identity is invalid: %w", err)
	}
	return nil
}

func newInstanceID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate discovery instance id: %w", err)
	}
	// RFC 4122 version 4, variant 1. The standard UUID textual form is useful
	// in logs and remains stable across all client implementations.
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return strings.Join([]string{
		encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32],
	}, "-"), nil
}
