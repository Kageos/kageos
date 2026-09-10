import { get, put, post } from '@/architecture/infrastructure/apiClient/request'
import type { UserInfo } from '@/architecture/domain/types'

// 更新用户信息
export interface UpdateUserReq {
  nickname?: string
  signature?: string
  avatar?: string
  gender?: 'male' | 'female' | 'other' | ''
}

export interface UpdateUserResp {
  user: UserInfo
}

/**
 * 更新当前登录用户信息
 */
export function updateUser(data: UpdateUserReq) {
  return put<UpdateUserResp>('/hr/api/v1/user/update', data)
}

export interface ChangeOwnPasswordReq {
  current_password: string
  new_password: string
}

export function changeOwnPassword(data: ChangeOwnPasswordReq) {
  return put('/hr/api/v1/user/password', data)
}

export interface OpenAPITokenInfo {
  id: number
  name: string
  token_prefix: string
  expires_at?: string
  revoked_at?: string
  last_used_at?: string
  last_used_ip?: string
  last_used_user_agent?: string
  created_at: string
}

export interface ListOpenAPITokensResp {
  tokens: OpenAPITokenInfo[]
}

export interface CreateOpenAPITokenReq {
  name: string
  expires_at?: string
}

export interface CreateOpenAPITokenResp {
  token: OpenAPITokenInfo
  secret_token: string
}

export function listOpenAPITokens() {
  return get<ListOpenAPITokensResp>('/hr/api/v1/user/openapi_tokens')
}

export function createOpenAPIToken(data: CreateOpenAPITokenReq) {
  return post<CreateOpenAPITokenResp>('/hr/api/v1/user/openapi_tokens', data)
}

export function revokeOpenAPIToken(id: number) {
  return post('/hr/api/v1/user/openapi_tokens/revoke', { id })
}

// 根据用户名精确查询
export interface QueryUserResp {
  user: UserInfo
}

/**
 * 根据用户名精确查询用户信息
 */
export function queryUser(username: string) {
  return get<QueryUserResp>('/hr/api/v1/user/query', { username })
}

// 模糊查询用户
export interface SearchUsersFuzzyResp {
  users: UserInfo[]
}

/**
 * 模糊查询用户（支持用户名和邮箱）
 * @param keyword 搜索关键词
 * @param limit 返回数量限制，默认10，最大100
 */
export function searchUsersFuzzy(keyword: string, limit: number = 10) {
  return get<SearchUsersFuzzyResp>('/hr/api/v1/user/search_fuzzy', { keyword, limit })
}

// 批量获取用户信息
export interface GetUsersByUsernamesReq {
  usernames: string[]
}

export interface GetUsersByUsernamesResp {
  users: UserInfo[]
}

/**
 * 根据用户名列表批量获取用户信息
 * @param usernames 用户名列表，最多100个
 */
export function getUsersByUsernames(usernames: string[]) {
  return post<GetUsersByUsernamesResp>('/hr/api/v1/users', { usernames })
}

// 根据部门获取用户列表
export interface GetUsersByDepartmentReq {
  department_full_path: string
}

export interface GetUsersByDepartmentResp {
  users: UserInfo[]
}

/**
 * 根据部门完整路径获取用户列表
 */
export function getUsersByDepartment(departmentFullPath: string) {
  return get<GetUsersByDepartmentResp>('/hr/api/v1/user/department', { department_full_path: departmentFullPath })
}

export interface SystemListUsersReq {
  keyword?: string
  status?: string
  register_type?: string
  page?: number
  page_size?: number
}

export interface SystemListUsersResp {
  users: UserInfo[]
  total: number
  page: number
  page_size: number
}

export interface SystemCreateUserReq {
  username: string
  password: string
  email?: string
  nickname?: string
  department_full_path?: string
  leader_username?: string
  status?: 'active' | 'pending' | 'disabled'
}

export interface SystemUserResp {
  user: UserInfo
}

export interface SystemUpdateUserReq {
  email?: string
  nickname?: string
  signature?: string
  avatar?: string
  gender?: string
  department_full_path?: string
  leader_username?: string
}

export function listSystemUsers(params: SystemListUsersReq = {}) {
  return get<SystemListUsersResp>('/hr/api/v1/system/users', params)
}

export function createSystemUser(data: SystemCreateUserReq) {
  return post<SystemUserResp>('/hr/api/v1/system/users', data)
}

export function updateSystemUser(username: string, data: SystemUpdateUserReq) {
  return put<SystemUserResp>(`/hr/api/v1/system/users/${encodeURIComponent(username)}`, data)
}

export function resetSystemUserPassword(username: string, password: string) {
  return post<SystemUserResp>(`/hr/api/v1/system/users/${encodeURIComponent(username)}/password`, { password })
}

export function updateSystemUserStatus(username: string, status: 'active' | 'pending' | 'disabled', reason = '') {
  return put<SystemUserResp>(`/hr/api/v1/system/users/${encodeURIComponent(username)}/status`, { status, reason })
}

export interface SystemImportUserResult { row: number; username: string; status: 'ready' | 'created' | 'failed'; message?: string }
export function importSystemUsers(rows: SystemCreateUserReq[], preview: boolean) {
  return post<{ rows: SystemImportUserResult[] }>('/hr/api/v1/system/users/import', { rows, preview })
}
