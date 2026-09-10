import { ElMessage } from 'element-plus'
import { nextTick, ref, type Ref } from 'vue'
import { useAuthStore } from '@/architecture/presentation/context/appStoresContext'
import { uploadFile, notifyUploadComplete, type UploadProgress } from '@/architecture/presentation/context/uploadContext'
import type { WorkspaceChatMessageFile } from '@/architecture/presentation/context/api/workspace'
import {
  normalizeWorkspaceResourcePath,
  parseWorkspacePromptSegments,
  resolveWorkspaceResourcePath,
  wrapWorkspaceResourcePath,
} from '@/architecture/presentation/components/utils/workspaceInvocationSnippet'

const UPLOAD_ROUTER = 'workspace/chat'

type ClipboardFileTransfer = Pick<DataTransfer, 'files' | 'items'>

interface WorkspaceNodeDropPayload {
  type?: string
  full_code_path?: string
  name?: string
}

export interface UseMiniWorkstationUploadsOptions {
  fullCodePath: Ref<string>
  inputText: Ref<string>
  inputRef: Ref<{
    focus: () => void
    focusAtEnd?: () => void
    insertWorkspaceResources?: (paths: string[]) => void
  } | undefined>
}

export function extractClipboardFiles(dataTransfer: ClipboardFileTransfer | null | undefined): File[] {
  if (!dataTransfer) {
    return []
  }

  const fileListFiles = Array.from(dataTransfer.files || [])
  if (fileListFiles.length > 0) {
    return fileListFiles
  }

  const itemFiles: File[] = []
  Array.from(dataTransfer.items || []).forEach((item) => {
    if (item.kind === 'file') {
      const file = item.getAsFile()
      if (file) {
        itemFiles.push(file)
      }
    }
  })

  return itemFiles
}

export function appendWorkspaceResourceTokens(currentText: string, paths: string[], basePath = ''): string {
  const existingPaths = new Set(
    parseWorkspacePromptSegments(currentText, basePath)
      .filter(segment => segment.type === 'resource')
      .map(segment => normalizeWorkspaceResourcePath(segment.path || ''))
      .filter(Boolean)
  )
  const tokens: string[] = []

  paths.forEach((path) => {
    const normalized = normalizeWorkspaceResourcePath(path)
    const resolved = normalizeWorkspaceResourcePath(resolveWorkspaceResourcePath(normalized, basePath))
    if (!normalized || !resolved || existingPaths.has(resolved)) {
      return
    }
    existingPaths.add(resolved)
    tokens.push(wrapWorkspaceResourcePath(normalized))
  })

  if (tokens.length === 0) {
    return currentText
  }
  const separator = currentText && !/\s$/.test(currentText) ? ' ' : ''
  return `${currentText}${separator}${tokens.join(' ')}`
}

function parseWorkspaceNodeDropPayload(raw: string): WorkspaceNodeDropPayload[] {
  const parsed = raw ? JSON.parse(raw) as WorkspaceNodeDropPayload | WorkspaceNodeDropPayload[] : null
  return (Array.isArray(parsed) ? parsed : [parsed])
    .filter((item): item is WorkspaceNodeDropPayload => Boolean(item?.full_code_path))
}

export function useMiniWorkstationUploads(options: UseMiniWorkstationUploadsOptions) {
  const { fullCodePath, inputText, inputRef } = options
  const authStore = useAuthStore()

  const attachedFiles = ref<WorkspaceChatMessageFile[]>([])
  const uploading = ref(false)
  const dragOver = ref(false)
  let dragLeaveTimer: ReturnType<typeof setTimeout> | null = null

  async function onFileChange(uploadFileObj: { raw?: File }) {
    const file = uploadFileObj?.raw
    if (!file || !fullCodePath.value) {
      return
    }

    uploading.value = true
    try {
      const uploadResult = await uploadFile(UPLOAD_ROUTER, file, (_progress: UploadProgress) => {})
      if (!uploadResult.fileInfo) {
        throw new Error('上传失败')
      }

      const completeResult = await notifyUploadComplete({
        key: uploadResult.fileInfo.key,
        bucket: uploadResult.fileInfo.bucket,
        success: true,
        router: uploadResult.fileInfo.router,
        file_name: uploadResult.fileInfo.file_name,
        file_size: uploadResult.fileInfo.file_size,
        content_type: uploadResult.fileInfo.content_type,
        hash: uploadResult.fileInfo.hash,
        upload_user: authStore.userName || undefined
      })

      if (!completeResult?.download_url) {
        throw new Error('获取下载地址失败')
      }

      attachedFiles.value.push({
        ref: completeResult.ref || uploadResult.fileInfo.ref,
        bucket: completeResult.bucket || uploadResult.fileInfo.bucket,
        key: uploadResult.fileInfo.key,
        name: completeResult.file_name,
        source_name: file.name,
        storage: completeResult.storage || uploadResult.storage,
        hash: completeResult.hash || uploadResult.fileInfo.hash || '',
        size: completeResult.file_size,
        upload_ts: Math.floor(Date.now() / 1000),
        is_uploaded: true,
        download_url: completeResult.download_url,
        server_download_url: completeResult.server_download_url,
        upload_user: authStore.userName || undefined
      })
      ElMessage.success(`已添加：${file.name}`)
    } catch (error: any) {
      ElMessage.error(error?.message || '上传失败')
    } finally {
      uploading.value = false
    }
  }

  async function onPaste(event: ClipboardEvent) {
    if (event.defaultPrevented) {
      return
    }

    const files = extractClipboardFiles(event.clipboardData)
    if (files.length === 0) {
      return
    }

    event.preventDefault()
    event.stopPropagation()

    if (!fullCodePath.value) {
      ElMessage.warning('请先选择服务目录后再粘贴文件')
      return
    }

    for (const file of files) {
      await onFileChange({ raw: file })
    }
  }

  function removeFile(index: number) {
    attachedFiles.value.splice(index, 1)
  }

  function onDragOver(_event: DragEvent) {
    if (dragLeaveTimer) {
      clearTimeout(dragLeaveTimer)
      dragLeaveTimer = null
    }
    dragOver.value = true
  }

  function onDragLeave(_event: DragEvent) {
    if (dragLeaveTimer) {
      clearTimeout(dragLeaveTimer)
    }
    dragLeaveTimer = setTimeout(() => {
      dragOver.value = false
    }, 80)
  }

  async function onDrop(event: DragEvent) {
    dragOver.value = false
    const dataTransfer = event.dataTransfer
    if (!dataTransfer) {
      return
    }

    if (dataTransfer.types.includes('application/x-workspace-node')) {
      try {
        const raw = dataTransfer.getData('application/x-workspace-node')
        const payloads = parseWorkspaceNodeDropPayload(raw)
        const paths = payloads
          .map(payload => payload.full_code_path || '')
          .filter(Boolean)
        if (paths.length > 0) {
          if (inputRef.value?.insertWorkspaceResources) {
            inputRef.value.insertWorkspaceResources(paths)
            return
          }
          inputText.value = appendWorkspaceResourceTokens(inputText.value, paths, fullCodePath.value)
          await nextTick()
          if (inputRef.value?.focusAtEnd) {
            inputRef.value.focusAtEnd()
          } else {
            inputRef.value?.focus()
          }
        }
      } catch {
        // ignore parse error
      }
      return
    }

    const files = dataTransfer.files
    if (!files?.length || !fullCodePath.value) {
      return
    }

    for (const file of Array.from(files)) {
      await onFileChange({ raw: file })
    }
  }

  return {
    attachedFiles,
    uploading,
    dragOver,
    onFileChange,
    removeFile,
    onDragOver,
    onDragLeave,
    onDrop,
    onPaste
  }
}
