package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// 用户分组限制（防蒸馏临时封禁）管理接口。详见 docs/channel/user-group-block.md

const maxUserGroupBlockNoteRunes = 255

type GroupBlockUserItem struct {
	Id          int                  `json:"id"`
	Username    string               `json:"username"`
	DisplayName string               `json:"display_name"`
	Group       string               `json:"group"`
	Blocks      []dto.UserGroupBlock `json:"blocks"`
}

type GroupBlockAvailableGroup struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// GetGroupBlockUsers 列出当前有生效中分组限制的用户（已全部过期的不列）
func GetGroupBlockUsers(c *gin.Context) {
	users, err := model.GetAllGroupBlockUsers()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := time.Now().Unix()
	result := make([]GroupBlockUserItem, 0, len(users))
	for _, user := range users {
		active := parseUserGroupBlocks(user.GroupBlocks).ActiveBlocks(now)
		if len(active) == 0 {
			continue
		}
		sortUserGroupBlocks(active)
		result = append(result, GroupBlockUserItem{
			Id:          user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Group:       user.Group,
			Blocks:      active,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Id < result[j].Id
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetUserGroupBlocks 返回单个用户的生效中限制 + 可选分组（用户可用分组，不含 auto）
func GetUserGroupBlocks(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	active := parseUserGroupBlocks(user.GroupBlocks).ActiveBlocks(time.Now().Unix())
	sortUserGroupBlocks(active)

	usable := service.GetUserUsableGroups(user.Group)
	groups := make([]GroupBlockAvailableGroup, 0, len(usable))
	for name, desc := range usable {
		if name == "auto" {
			continue
		}
		groups = append(groups, GroupBlockAvailableGroup{Name: name, Desc: desc})
	}
	// 已封但不在用户可用分组里的（可用分组配置后来改过）也列出，便于解除
	for _, block := range active {
		if _, ok := usable[block.Group]; !ok {
			groups = append(groups, GroupBlockAvailableGroup{Name: block.Group})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":           user.Id,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"group":        user.Group,
			"blocks":       active,
			"groups":       groups,
		},
	})
}

// UpdateUserGroupBlocks 整体覆盖用户的分组限制：请求里没有的分组即解除。
// 原本就在生效中且到期时间未变的分组保留原创建时间与操作人。
func UpdateUserGroupBlocks(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	var req dto.UserGroupBlocksUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := time.Now().Unix()
	existing := parseUserGroupBlocks(user.GroupBlocks)
	seen := make(map[string]bool, len(req.Blocks))
	blocks := make([]dto.UserGroupBlock, 0, len(req.Blocks))
	for _, item := range req.Blocks {
		group := strings.TrimSpace(item.Group)
		note := strings.TrimSpace(item.Note)
		if msg := validateUserGroupBlockItem(group, item.ExpiresAt, note, now, seen); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": msg,
			})
			return
		}
		seen[group] = true

		block := dto.UserGroupBlock{
			Group:     group,
			ExpiresAt: item.ExpiresAt,
			Note:      note,
			CreatedBy: c.GetInt("id"),
			CreatedAt: now,
		}
		if prev, ok := existing.ActiveBlock(group, now); ok && prev.ExpiresAt == item.ExpiresAt {
			block.CreatedBy = prev.CreatedBy
			block.CreatedAt = prev.CreatedAt
		}
		blocks = append(blocks, block)
	}

	if err := saveUserGroupBlocks(id, blocks); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteUserGroupBlocks 解除用户的全部分组限制
func DeleteUserGroupBlocks(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if err := saveUserGroupBlocks(id, nil); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func validateUserGroupBlockItem(group string, expiresAt int64, note string, now int64, seen map[string]bool) string {
	if group == "" {
		return "分组不能为空"
	}
	if group == "auto" {
		return "不能限制 auto 分组，请直接限制具体分组"
	}
	if !ratio_setting.ContainsGroupRatio(group) {
		return "分组 " + group + " 不存在"
	}
	if seen[group] {
		return "分组 " + group + " 重复"
	}
	if expiresAt != 0 && expiresAt <= now {
		return "分组 " + group + " 的到期时间必须晚于当前时间"
	}
	if utf8.RuneCountInString(note) > maxUserGroupBlockNoteRunes {
		return "备注过长"
	}
	return ""
}

// saveUserGroupBlocks 写库并清用户缓存；blocks 为空时清空该列
func saveUserGroupBlocks(userId int, blocks []dto.UserGroupBlock) error {
	if len(blocks) == 0 {
		return model.UpdateUserGroupBlocks(userId, "")
	}
	blocksJSON, err := common.Marshal(dto.UserGroupBlocks{Blocks: blocks})
	if err != nil {
		return err
	}
	return model.UpdateUserGroupBlocks(userId, string(blocksJSON))
}

func parseUserGroupBlocks(blocksJSON string) dto.UserGroupBlocks {
	blocks := dto.UserGroupBlocks{}
	if blocksJSON != "" {
		_ = common.UnmarshalJsonStr(blocksJSON, &blocks)
	}
	return blocks
}

func sortUserGroupBlocks(blocks []dto.UserGroupBlock) {
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Group < blocks[j].Group
	})
}
