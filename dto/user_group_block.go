package dto

// UserGroupBlocks is the per-user group restriction stored on the user row
// (users.group_blocks). Admins use it to temporarily stop a user from using
// specific groups (e.g. when distillation is detected) without affecting the
// user's other groups. Expired entries are ignored at request time.
type UserGroupBlocks struct {
	Blocks []UserGroupBlock `json:"blocks,omitempty"`
}

type UserGroupBlock struct {
	Group string `json:"group"`
	// ExpiresAt is a unix timestamp in seconds; 0 means permanent.
	ExpiresAt int64 `json:"expires_at"`
	// Note is an admin-only internal remark, never shown to the user.
	Note      string `json:"note,omitempty"`
	CreatedBy int    `json:"created_by,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// IsActive reports whether the block is in effect at unix time now.
func (b UserGroupBlock) IsActive(now int64) bool {
	return b.ExpiresAt == 0 || b.ExpiresAt > now
}

// ActiveBlock returns the block in effect for group at unix time now.
func (b UserGroupBlocks) ActiveBlock(group string, now int64) (UserGroupBlock, bool) {
	for _, block := range b.Blocks {
		if block.Group == group && block.IsActive(now) {
			return block, true
		}
	}
	return UserGroupBlock{}, false
}

// ActiveBlocks returns all blocks in effect at unix time now.
func (b UserGroupBlocks) ActiveBlocks(now int64) []UserGroupBlock {
	active := make([]UserGroupBlock, 0, len(b.Blocks))
	for _, block := range b.Blocks {
		if block.IsActive(now) {
			active = append(active, block)
		}
	}
	return active
}

// UserGroupBlockItem is one group to block in an admin write.
// ExpiresAt follows UserGroupBlock: unix seconds, 0 = permanent.
type UserGroupBlockItem struct {
	Group     string `json:"group"`
	ExpiresAt int64  `json:"expires_at"`
	Note      string `json:"note,omitempty"`
}

// UserGroupBlocksUpdateRequest is the admin write payload: the full set of
// active blocks for the user (groups omitted here are unblocked).
type UserGroupBlocksUpdateRequest struct {
	Blocks []UserGroupBlockItem `json:"blocks"`
}
