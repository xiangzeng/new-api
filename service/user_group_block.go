package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 用户分组限制（防蒸馏临时封禁）：管理员对单个用户封禁某些分组，到期自动恢复。
// 指定分组命中封禁由 distributor 明示拒绝；auto 分组跳过被封分组顺延到下一组。
// 到期在请求时按当前时间判断，无定时任务。详见 docs/channel/user-group-block.md

// requestUserGroupBlocks 当前请求用户的分组限制配置。令牌鉴权已由 UserBase.WriteContext
// 写入 context；游乐场走会话鉴权不写，按用户 id 读用户缓存补齐并回写 context。
func requestUserGroupBlocks(c *gin.Context) dto.UserGroupBlocks {
	if c == nil {
		return dto.UserGroupBlocks{}
	}
	if blocks, ok := common.GetContextKeyType[dto.UserGroupBlocks](c, constant.ContextKeyUserGroupBlocks); ok {
		return blocks
	}
	blocks := dto.UserGroupBlocks{}
	if userId := c.GetInt(string(constant.ContextKeyUserId)); userId > 0 {
		if userCache, err := model.GetUserCache(userId); err == nil {
			blocks = userCache.GetGroupBlocks()
		}
	}
	common.SetContextKey(c, constant.ContextKeyUserGroupBlocks, blocks)
	return blocks
}

// GetUserGroupBlock 当前请求用户在分组 group 上生效中的封禁
func GetUserGroupBlock(c *gin.Context, group string) (dto.UserGroupBlock, bool) {
	return requestUserGroupBlocks(c).ActiveBlock(group, time.Now().Unix())
}

// GetRequestUserAutoGroup 当前请求可用的 auto 分组列表：在 GetRequestAutoGroups（含令牌级 auto 分组）
// 基础上去掉被封分组。选路与重试上限用它；定价页、模型列表等展示用途仍用 GetUserAutoGroup。
func GetRequestUserAutoGroup(c *gin.Context, userGroup string) []string {
	autoGroups := GetRequestAutoGroups(c, userGroup)
	blocks := requestUserGroupBlocks(c)
	if len(blocks.Blocks) == 0 {
		return autoGroups
	}
	now := time.Now().Unix()
	usable := make([]string, 0, len(autoGroups))
	for _, group := range autoGroups {
		if _, blocked := blocks.ActiveBlock(group, now); !blocked {
			usable = append(usable, group)
		}
	}
	return usable
}

// CheckUserGroupBlocked 判断本次请求是否因分组限制整体不可用：
// 指定分组被封，或 auto 下用户的全部 auto 分组都被封。
// expiresAt 为最早恢复时间（unix 秒），0 表示永久。
func CheckUserGroupBlocked(c *gin.Context, usingGroup string) (blocked bool, expiresAt int64) {
	blocks := requestUserGroupBlocks(c)
	if len(blocks.Blocks) == 0 {
		return false, 0
	}
	now := time.Now().Unix()
	if usingGroup != "auto" {
		block, ok := blocks.ActiveBlock(usingGroup, now)
		if !ok {
			return false, 0
		}
		return true, block.ExpiresAt
	}

	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	autoGroups := GetRequestAutoGroups(c, userGroup)
	if len(autoGroups) == 0 {
		return false, 0
	}
	// 任一分组先解封，auto 即可用：取非永久封禁里最早的到期时间
	earliest := int64(0)
	for _, group := range autoGroups {
		block, ok := blocks.ActiveBlock(group, now)
		if !ok {
			return false, 0
		}
		if block.ExpiresAt > 0 && (earliest == 0 || block.ExpiresAt < earliest) {
			earliest = block.ExpiresAt
		}
	}
	return true, earliest
}
