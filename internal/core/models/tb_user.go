package models

import (
	"golang.org/x/crypto/bcrypt"
	"time"
)

const TableNameTbUser = "tb_user"

type TBUser struct {
	Id        string    `gorm:"column:id;primaryKey;comment:主键" json:"id"` // 参数主键
	Username  string    `gorm:"size:500;column:user_name;comment:登录名" json:"user_name"`
	Email     string    `gorm:"size:255;column:email;comment:邮箱" json:"email"`
	PassWord  string    `gorm:"size:255;column:pass_word;comment:视频id" json:"pass_word"` // 参数主键
	NickName  string    `gorm:"size:64;column:nick_name;comment:昵称" json:"nick_name"`    // 频道ID
	Status    string    `gorm:"size:10;column:status;comment:状态" json:"status"`
	VipExpire time.Time `gorm:"column:vip_expire;comment:会员到期时间" json:"vip_expire"`    // 会员到期时间
	Phone     string    `gorm:"size:20;column:phone;comment:手机号" json:"phone"`         // 手机号
	Avatar    string    `gorm:"size:255;column:avatar;comment:头像" json:"avatar"`       // gorm:"-" 表示该字段不会映射到数据库
	IsVip     bool      `gorm:"size:6;column:is_vip;comment: 是否 VIP 会员" json:"is_vip"` // 是否 VIP 会员
	Platform  string    `gorm:"size:255;column:platform;comment://平台" json:"platform"` //平台
	Credit    int64     `gorm:"size:255;column:credit;comment:积分" json:"credit"`       //积分

	CreateBy   string    `gorm:"size:32;column:create_by;comment:创建者" json:"create_by"` // 创建者
	CreateTime time.Time `gorm:"column:create_time;comment:创建时间" json:"create_time"`    // 创建时间
	UpdateBy   string    `gorm:"size:32;column:update_by;comment:更新者" json:"update_by"` // 更新者
	UpdateTime time.Time `gorm:"column:update_time;comment:更新时间" json:"update_time"`    // 更新时间
	Remark     string    `gorm:"size:255;column:remark;comment:备注" json:"remark"`
}

func (*TBUser) TableName() string {
	return TableNameTbUser
}

// HashPassword 对密码进行哈希加密
func (user *TBUser) HashPassword(password string) error {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}
	user.PassWord = string(bytes)
	return nil
}

// CheckPassword 验证密码
func (user *TBUser) CheckPassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(user.PassWord), []byte(password))
}
