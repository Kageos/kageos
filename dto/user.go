package dto

// QueryUserReq 查询用户请求
type QueryUserReq struct {
	Username string `json:"username" form:"username" binding:"required" example:"beiluo"` // 用户名
}

// QueryUserResp 查询用户响应
type QueryUserResp struct {
	User UserInfo `json:"user"` // 用户信息
}

// SearchUsersFuzzyReq 模糊查询用户请求
type SearchUsersFuzzyReq struct {
	Keyword string `json:"keyword" form:"keyword" binding:"required" example:"bei"` // 搜索关键词
	Limit   int    `json:"limit" form:"limit" example:"10"`                         // 返回数量限制，默认10，最大100
}

// SearchUsersFuzzyResp 模糊查询用户响应
type SearchUsersFuzzyResp struct {
	Users []UserInfo `json:"users"` // 用户列表
}

// GetUsersByUsernamesReq 批量获取用户请求
type GetUsersByUsernamesReq struct {
	Usernames []string `json:"usernames" binding:"required,min=1,max=100" example:"[\"user1\",\"user2\"]"` // 用户名列表，最多100个
}

// GetUsersByUsernamesResp 批量获取用户响应
type GetUsersByUsernamesResp struct {
	Users []UserInfo `json:"users"` // 用户列表
}

// UpdateUserReq 更新用户信息请求（所有字段都是可选的）
type UpdateUserReq struct {
	Nickname  *string `json:"nickname,omitempty" example:"北落"`                     // 昵称（可选，传值则更新，不传则不更新）
	Signature *string `json:"signature,omitempty" example:"这个人很懒，什么都没有留下"`         // 个人签名/简介（可选，传值则更新，不传则不更新）
	Avatar    *string `json:"avatar,omitempty" example:"https://avatar.com/1.jpg"` // 头像URL（可选，传值则更新，不传则不更新）
	Gender    *string `json:"gender,omitempty" example:"male"`                     // 性别（可选，传值则更新，不传则不更新）: male(男), female(女), other(其他)
}

// UpdateUserResp 更新用户信息响应
type UpdateUserResp struct {
	User UserInfo `json:"user"` // 更新后的用户信息
}

// SystemListUsersReq system 用户管理：分页查询用户
type SystemListUsersReq struct {
	Keyword      string `json:"keyword" form:"keyword" example:"bei"`  // 搜索关键词：用户名、邮箱、昵称
	Status       string `json:"status" form:"status" example:"active"` // 用户状态：active/pending/disabled
	RegisterType string `json:"register_type" form:"register_type" example:"email"`
	Page         int    `json:"page" form:"page" example:"1"`
	PageSize     int    `json:"page_size" form:"page_size" example:"20"`
}

// SystemListUsersResp system 用户管理：分页用户列表
type SystemListUsersResp struct {
	Users    []UserInfo `json:"users"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// SystemCreateUserReq system 用户管理：创建用户
type SystemCreateUserReq struct {
	Username           string `json:"username" binding:"required,min=3,max=32" example:"newuser"`
	Password           string `json:"password" binding:"required,min=6" example:"123456"`
	Email              string `json:"email" example:"newuser@example.com"`
	Nickname           string `json:"nickname" example:"新用户"`
	DepartmentFullPath string `json:"department_full_path" example:"/org/unassigned"`
	LeaderUsername     string `json:"leader_username" example:"leader1"`
	Status             string `json:"status" example:"active"` // 为空默认 active；支持 active/pending/disabled
}

// SystemUserResp system 用户管理：单用户响应
type SystemUserResp struct {
	User UserInfo `json:"user"`
}

// SystemUpdateUserReq system 用户管理：更新用户基础信息
type SystemUpdateUserReq struct {
	Email              *string `json:"email,omitempty" example:"newuser@example.com"`
	Nickname           *string `json:"nickname,omitempty" example:"新用户"`
	Signature          *string `json:"signature,omitempty" example:"负责客户成功"`
	Avatar             *string `json:"avatar,omitempty" example:"https://avatar.com/1.jpg"`
	Gender             *string `json:"gender,omitempty" example:"other"`
	DepartmentFullPath *string `json:"department_full_path,omitempty" example:"/org/unassigned"`
	LeaderUsername     *string `json:"leader_username,omitempty" example:"leader1"`
}

// SystemResetUserPasswordReq system 用户管理：重置用户密码
type SystemResetUserPasswordReq struct {
	Password string `json:"password" binding:"required,min=6" example:"123456"`
}

// ChangeOwnPasswordReq 当前登录用户修改自己的密码
type ChangeOwnPasswordReq struct {
	CurrentPassword string `json:"current_password" binding:"required" example:"old-password"`
	NewPassword     string `json:"new_password" binding:"required,min=6" example:"new-password"`
}

// SystemUpdateUserStatusReq system 用户管理：更新用户状态
type SystemUpdateUserStatusReq struct {
	Reason string `json:"reason" binding:"max=500"`
	Status string `json:"status" binding:"required,oneof=active pending disabled" example:"disabled"`
}

// SystemImportUsersReq contains bounded import rows; preview never writes accounts.
type SystemImportUsersReq struct {
	Rows    []SystemCreateUserReq `json:"rows" binding:"required,min=1,max=100"`
	Preview bool                  `json:"preview"`
}
type SystemImportUserResult struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}
