import { get, post, put, del } from '@/architecture/infrastructure/apiClient/request'
import type { CapabilityBundle, ServiceTree, CreateServiceTreeRequest, FunctionConnectorEndpoint, FunctionSchema } from '@/architecture/domain/types'
import type { TimerTask } from './timer'

// ⭐ 创建 package 类型节点（推荐使用）
export function createPackage(data: CreateServiceTreeRequest) {
  const payload = {
    user: data.user,
    app: data.app,
    name: data.name,
    code: data.code,
    parent_full_code_path: data.parent_full_code_path || '',
    description: data.description || '',
    tags: data.tags || '',
    admins: data.admins || ''
  }
  return post<ServiceTree>('/workspace/api/v1/packages', payload)
}

// ⭐ 创建 docs 类型节点（推荐使用）
export function createDocs(data: CreateServiceTreeRequest & {
  content?: string
  format?: string
  summary?: string
}) {
  const payload: any = {
    user: data.user,
    app: data.app,
    name: data.name,
    code: data.code,
    parent_full_code_path: data.parent_full_code_path || '',
    description: data.description || '',
    tags: data.tags || '',
    admins: data.admins || ''
  }
  
  if (data.content) {
    payload.content = data.content
  }
  if (data.format) {
    payload.format = data.format
  }
  if (data.summary) {
    payload.summary = data.summary
  }
  
  return post<ServiceTree>('/workspace/api/v1/docs/crud', payload)
}

// ⭐ 创建 function 类型节点（推荐使用）
// 注意：为了避免与 function.ts 中的 createFunction 冲突，这里命名为 createServiceTreeFunction
export function createServiceTreeFunction(data: {
  user: string
  app: string
  name: string
  code: string
  directory_path: string
  template_type?: string
  source_code: string
  description?: string
  tags?: string
}) {
  const payload = {
    user: data.user,
    app: data.app,
    name: data.name,
    code: data.code,
    directory_path: data.directory_path,
    template_type: data.template_type || '',
    source_code: data.source_code,
    description: data.description || '',
    tags: data.tags || ''
  }
  return post<ServiceTree>('/workspace/api/v1/functions', payload)
}

// ⭐ 更新 package 类型节点（推荐使用）
export function updatePackage(id: number, data: { name?: string; code?: string; description?: string; tags?: string; admins?: string }) {
  return put(`/workspace/api/v1/packages/${id}`, data)
}

// ⭐ 删除 package 类型节点（推荐使用）
export function deletePackage(id: number) {
  return del(`/workspace/api/v1/packages/${id}`)
}

// ⭐ 更新 function 类型节点（推荐使用）
// 注意：为了避免与 function.ts 中的 updateFunction 冲突，这里命名为 updateServiceTreeFunction
export function updateServiceTreeFunction(id: number, data: { name?: string; code?: string; description?: string; tags?: string }) {
  return put(`/workspace/api/v1/functions/${id}`, data)
}

// ⭐ 删除 function 类型节点（推荐使用）
// 注意：为了避免与 function.ts 中的 deleteFunction 冲突，这里命名为 deleteServiceTreeFunction
export function deleteServiceTreeFunction(id: number) {
  return del(`/workspace/api/v1/functions/${id}`)
}

// ⭐ 更新 docs 类型节点（推荐使用）
export function updateDocs(id: number, data: { name?: string; code?: string; description?: string; tags?: string; admins?: string; content?: string; format?: string; summary?: string }) {
  return put(`/workspace/api/v1/docs/crud/${id}`, data)
}

// ⭐ 删除 docs 类型节点（推荐使用）
export function deleteDocs(id: number) {
  return del(`/workspace/api/v1/docs/crud/${id}`)
}

export interface ServiceTreeDetailResp extends ServiceTree {
  version?: string
  version_num?: number
}

export function getServiceTreeDetail(fullCodePath: string) {
  return get<ServiceTreeDetailResp>('/workspace/api/v1/service_tree/detail', {
    full_code_path: fullCodePath
  })
}

export interface BatchGetServiceTreeDetailsReq {
  full_code_paths: string[]
}

export interface BatchGetServiceTreeDetailsResp {
  items: ServiceTreeDetailResp[]
  missing?: string[]
}

export function batchGetServiceTreeDetails(req: BatchGetServiceTreeDetailsReq) {
  return post<BatchGetServiceTreeDetailsResp>('/workspace/api/v1/service_tree/batch_detail', {
    full_code_paths: req.full_code_paths || []
  })
}

export interface DirectoryOverviewResource {
  id?: number
  name?: string
  code?: string
  type?: string
  full_code_path?: string
  template_type?: string
  run_count?: number
}

export interface DirectoryOverviewStats {
  directories: number
  functions: number
  docs: number
  total_run_count: number
  scheduled_function_tasks: number
  scheduled_agent_tasks: number
  running_tasks: number
  failed_tasks: number
  paused_tasks: number
  next_run_at?: string
}

export interface DirectoryOverviewScheduledTask {
  kind: 'function' | 'agent'
  origin: 'manifest' | 'user'
  builtin: boolean
  resource?: DirectoryOverviewResource
  resource_path?: string
  resource_name?: string
  task: TimerTask
}

export interface DirectoryOverviewResp {
  directory?: DirectoryOverviewResource
  stats: DirectoryOverviewStats
  scheduled_function_tasks: DirectoryOverviewScheduledTask[]
  scheduled_agent_tasks: DirectoryOverviewScheduledTask[]
  partial?: boolean
  warnings?: string[]
}

export function getDirectoryOverview(fullCodePath: string) {
  return get<DirectoryOverviewResp>('/workspace/api/v1/service_tree/overview', {
    full_code_path: fullCodePath
  })
}

// 复制服务目录（新接口，支持递归复制）
export function copyDirectory(data: {
  source_directory_path: string
  target_directory_path: string
  target_app_id: number
  target_directory_name?: string
  replace_existing?: boolean
}) {
  return post<{
    message: string
    directory_count: number
    file_count: number
    doc_count?: number
    scheduled_function_count?: number
    agent_task_count?: number
    replaced?: boolean
    target_directory_path?: string
    old_version?: string
    new_version?: string
    git_commit_hash?: string
  }>('/workspace/api/v1/service_tree/copy', data)
}

export function exportCapabilityBundle(data: {
  source_directory_path?: string
  source_directory_paths?: string[]
  source_root_path?: string
  name?: string
  include_user_task_ids?: number[]
}) {
  return post<CapabilityBundle>('/workspace/api/v1/service_tree/export_capability_bundle', data)
}

export function installCapabilityBundle(data: {
  target_directory_path: string
  overwrite?: boolean
  force_diff?: boolean
  bundle_subpath?: string
  bundle: CapabilityBundle
}) {
  return post<{
    message: string
    directory_count: number
    file_count: number
    doc_count?: number
    agent_task_count?: number
    target_directory_path: string
    created_paths?: string[]
    written_paths?: string[]
    old_version?: string
    new_version?: string
    warnings?: string[]
  }>('/workspace/api/v1/service_tree/install_capability_bundle', data)
}

export function installCapabilityBundleFromURL(data: {
  target_directory_path: string
  overwrite?: boolean
  force_diff?: boolean
  bundle_subpath?: string
  bundle_url: string
  install_key?: string
}) {
  return post<{
    message: string
    directory_count: number
    file_count: number
    doc_count?: number
    agent_task_count?: number
    target_directory_path: string
    created_paths?: string[]
    written_paths?: string[]
    old_version?: string
    new_version?: string
    warnings?: string[]
  }>('/workspace/api/v1/service_tree/install_capability_bundle_from_url', data)
}

// 搜索函数
export interface SearchFunctionsReq {
  full_code_path?: string
  user: string
  app: string
  keyword?: string
  template_type?: string
  page: number
  page_size: number
}

export interface FunctionSearchResult {
  id: number
  name: string
  code: string
  full_code_path: string
  description: string
  template_type: string
  app_id: number
  app_user: string
  app_code: string
  /** 函数 Schema，包含 form/table/chart 的字段结构 */
  schema?: FunctionSchema
  /** 函数级回调能力 */
  callbacks?: string[]
  /** 函数依赖的连接器 provider 列表 */
  connectors?: string[]
  /** 函数声明使用的连接器 API 端点 */
  connector_endpoints?: FunctionConnectorEndpoint[]
}

export interface SearchFunctionsResp {
  functions: FunctionSearchResult[]
  total: number
  page: number
  page_size: number
}

export function searchFunctions(req: SearchFunctionsReq) {
  return get<SearchFunctionsResp>('/workspace/api/v1/service_tree/search_functions', {
    full_code_path: req.full_code_path || '',
    user: req.user,
    app: req.app,
    keyword: req.keyword || '',
    template_type: req.template_type || '',
    page: req.page.toString(),
    page_size: req.page_size.toString()
  })
}

// 全站资源搜索
export type SearchResourceType = 'all' | 'package' | 'function' | 'docs'

export interface SearchResourcesReq {
  full_code_path?: string
  user?: string
  app?: string
  keyword: string
  resource_type?: SearchResourceType
  page?: number
  page_size?: number
}

export interface ResourceSearchResult {
  id: number
  name: string
  code: string
  type: 'package' | 'function' | 'docs'
  full_code_path: string
  description?: string
  tags?: string
  template_type?: string
  app_id?: number
  app_user?: string
  app_code?: string
  run_count?: number
  match_source?: string
  snippet?: string
}

export interface SearchResourcesResp {
  items: ResourceSearchResult[]
  total: number
  page: number
  page_size: number
}

export function searchResources(req: SearchResourcesReq) {
  return get<SearchResourcesResp>('/workspace/api/v1/service_tree/search_resources', {
    full_code_path: req.full_code_path || '',
    user: req.user || '',
    app: req.app || '',
    keyword: req.keyword || '',
    resource_type: req.resource_type || 'all',
    page: String(req.page || 1),
    page_size: String(req.page_size || 20)
  })
}
