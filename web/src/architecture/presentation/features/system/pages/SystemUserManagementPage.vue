<template>
  <div class="system-user-page">
    <div class="user-toolbar">
      <el-form :inline="true" :model="filters" class="filter-form">
        <el-form-item :label="t('systemUser.keyword')">
          <el-input
            v-model="filters.keyword"
            :prefix-icon="Search"
            clearable
            :placeholder="t('systemUser.keywordPlaceholder')"
            @keyup.enter="handleSearch"
          />
        </el-form-item>
        <el-form-item :label="t('systemUser.status')">
          <el-select v-model="filters.status" clearable class="filter-select">
            <el-option :label="t('systemUser.statusActive')" value="active" />
            <el-option :label="t('systemUser.statusPending')" value="pending" />
            <el-option :label="t('systemUser.statusDisabled')" value="disabled" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :icon="Search" @click="handleSearch">{{ t('common.search') }}</el-button>
          <el-button :icon="Refresh" :loading="loading" @click="loadUsers">{{ t('common.refresh') }}</el-button>
        </el-form-item>
      </el-form>
      <el-button @click="importVisible = true">{{ t('userImport.title') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openCreateDialog">
        {{ t('systemUser.create') }}
      </el-button>
    </div>

    <SystemUserImportDialog v-model="importVisible" @created="loadUsers" />
    <div class="user-summary">
      <div class="summary-item">
        <span class="summary-value">{{ total }}</span>
        <span class="summary-label">{{ t('systemUser.totalUsers') }}</span>
      </div>
      <div class="summary-item">
        <span class="summary-value">{{ activeUsersOnPage }}</span>
        <span class="summary-label">{{ t('systemUser.activeOnPage') }}</span>
      </div>
      <div class="summary-item">
        <span class="summary-value">{{ disabledUsersOnPage }}</span>
        <span class="summary-label">{{ t('systemUser.disabledOnPage') }}</span>
      </div>
    </div>

    <el-table
      v-loading="loading"
      :data="users"
      stripe
      class="user-table"
      :empty-text="t('systemUser.empty')"
    >
      <el-table-column :label="t('systemUser.user')" min-width="210">
        <template #default="{ row }">
          <div class="user-cell">
            <UserAvatar :size="32" :src="row.avatar" :alt="row.nickname || row.username" />
            <div class="user-cell-main">
              <span class="username">{{ row.username }}</span>
              <span class="nickname">{{ row.nickname || '-' }}</span>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="email" :label="t('systemUser.email')" min-width="210" show-overflow-tooltip />
      <el-table-column :label="t('systemUser.status')" width="120">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)">
            {{ statusLabel(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="register_type" :label="t('systemUser.registerType')" width="120" />
      <el-table-column :label="t('systemUser.department')" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.department_full_name_path || row.department_full_path || '-' }}
        </template>
      </el-table-column>
      <el-table-column :label="t('systemUser.createdAt')" width="170">
        <template #default="{ row }">
          {{ formatDateTime(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column
        :label="t('common.operation')"
        width="112"
        fixed="right"
        align="right"
        class-name="operation-column"
      >
        <template #default="{ row }">
          <div class="operation-cell">
            <el-tooltip :content="t('common.edit')" placement="top">
              <el-button
                class="operation-icon-button"
                text
                type="primary"
                :icon="EditPen"
                :aria-label="t('common.edit')"
                @click="openEditDialog(row)"
              />
            </el-tooltip>

            <el-dropdown
              trigger="click"
              popper-class="system-user-action-dropdown"
              @command="(command: string) => handleUserActionCommand(row, command)"
            >
              <el-button
                class="operation-icon-button operation-icon-button--more"
                text
                :icon="MoreFilled"
                :aria-label="t('systemUser.moreActions')"
              />
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="reset-password">
                    <span class="operation-menu-item">
                      <el-icon><Key /></el-icon>
                      <span>{{ t('systemUser.resetPassword') }}</span>
                    </span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    command="active"
                    divided
                    :disabled="isStatusActionDisabled(row, 'active')"
                  >
                    <span class="operation-menu-item">
                      <el-icon><CircleCheck /></el-icon>
                      <span>{{ t('systemUser.statusActive') }}</span>
                    </span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    command="pending"
                    :disabled="isStatusActionDisabled(row, 'pending')"
                  >
                    <span class="operation-menu-item">
                      <el-icon><Clock /></el-icon>
                      <span>{{ t('systemUser.statusPending') }}</span>
                    </span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    command="disabled"
                    :disabled="isStatusActionDisabled(row, 'disabled')"
                  >
                    <span class="operation-menu-item operation-menu-item--danger">
                      <el-icon><CircleClose /></el-icon>
                      <span>{{ t('systemUser.statusDisabled') }}</span>
                    </span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <div class="pagination-row">
      <el-pagination
        v-model:current-page="filters.page"
        v-model:page-size="filters.page_size"
        background
        layout="total, sizes, prev, pager, next"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        @current-change="loadUsers"
        @size-change="handlePageSizeChange"
      />
    </div>

    <el-dialog v-model="userDialogVisible" :title="userDialogTitle" width="720px">
      <el-form label-width="120px" class="user-form">
        <el-form-item :label="t('systemUser.username')" required>
          <el-input
            :model-value="userForm.username"
            :disabled="userDialogMode === 'edit'"
            placeholder="zhangsan"
            @update:model-value="normalizeUserCodeInput"
          />
        </el-form-item>
        <el-form-item v-if="userDialogMode === 'create'" :label="t('systemUser.password')" required>
          <el-input v-model="userForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('systemUser.nickname')">
          <el-input v-model="userForm.nickname" />
        </el-form-item>
        <el-form-item :label="t('systemUser.email')">
          <el-input v-model="userForm.email" placeholder="user@example.com" />
        </el-form-item>
        <el-form-item v-if="userDialogMode === 'create'" :label="t('systemUser.status')">
          <el-select v-model="userForm.status">
            <el-option :label="t('systemUser.statusActive')" value="active" />
            <el-option :label="t('systemUser.statusPending')" value="pending" />
            <el-option :label="t('systemUser.statusDisabled')" value="disabled" />
          </el-select>
        </el-form-item>
        <el-collapse v-model="userAdvancedPanels" class="user-advanced-settings">
          <el-collapse-item name="organization" :title="t('systemUser.organizationAdvanced')">
            <el-form-item :label="t('systemUser.departmentPath')">
              <el-input v-model="userForm.department_full_path" placeholder="/org/unassigned" />
            </el-form-item>
            <el-form-item :label="t('systemUser.leader')">
              <el-input v-model="userForm.leader_username" placeholder="leader_username" />
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="userDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="savingUser" @click="saveUser">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="passwordDialogVisible" :title="t('systemUser.resetPassword')" width="480px">
      <el-form label-width="110px">
        <el-form-item :label="t('systemUser.username')">
          <el-input :model-value="selectedUsername" disabled />
        </el-form-item>
        <el-form-item :label="t('systemUser.newPassword')" required>
          <el-input v-model="passwordForm.password" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="resettingPassword" @click="resetPassword">
          {{ t('systemUser.resetPassword') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CircleCheck, CircleClose, Clock, EditPen, Key, MoreFilled, Plus, Refresh, Search } from '@element-plus/icons-vue'
import type { UserInfo } from '@/architecture/domain/types'
import SystemUserImportDialog from '../components/SystemUserImportDialog.vue'
import UserAvatar from '@/architecture/presentation/shared/components/UserAvatar.vue'
import {
  createSystemUser,
  listSystemUsers,
  resetSystemUserPassword,
  updateSystemUser,
  updateSystemUserStatus,
} from '@/architecture/presentation/context/api/user'

const importVisible = ref(false)
const { t } = useI18n()

const loading = ref(false)
const savingUser = ref(false)
const resettingPassword = ref(false)
const userDialogVisible = ref(false)
const passwordDialogVisible = ref(false)
const userDialogMode = ref<'create' | 'edit'>('create')
const selectedUsername = ref('')
const userAdvancedPanels = ref<string[]>([])
const users = ref<UserInfo[]>([])
const total = ref(0)

const filters = reactive({
  keyword: '',
  status: '',
  register_type: '',
  page: 1,
  page_size: 20,
})

const userForm = reactive({
  username: '',
  password: '',
  email: '',
  nickname: '',
  department_full_path: '',
  leader_username: '',
  status: 'active' as 'active' | 'pending' | 'disabled',
})

const passwordForm = reactive({
  password: '',
})

const userDialogTitle = computed(() => {
  return userDialogMode.value === 'create' ? t('systemUser.createUser') : t('systemUser.editUser')
})
const activeUsersOnPage = computed(() => users.value.filter((user) => user.status === 'active').length)
const disabledUsersOnPage = computed(() => users.value.filter((user) => user.status === 'disabled').length)

async function loadUsers() {
  loading.value = true
  try {
    const resp = await listSystemUsers({
      keyword: filters.keyword,
      status: filters.status,
      register_type: filters.register_type,
      page: filters.page,
      page_size: filters.page_size,
    })
    users.value = resp.users || []
    total.value = resp.total || 0
    filters.page = resp.page || filters.page
    filters.page_size = resp.page_size || filters.page_size
  } catch (error: any) {
    ElMessage.error(error?.message || t('systemUser.loadFailed'))
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  filters.page = 1
  loadUsers()
}

function handlePageSizeChange() {
  filters.page = 1
  loadUsers()
}

function resetUserForm() {
  userForm.username = ''
  userForm.password = ''
  userForm.email = ''
  userForm.nickname = ''
  userForm.department_full_path = '/org/unassigned'
  userForm.leader_username = ''
  userForm.status = 'active'
}

function openCreateDialog() {
  resetUserForm()
  userAdvancedPanels.value = []
  userDialogMode.value = 'create'
  userDialogVisible.value = true
}

function openEditDialog(user: UserInfo) {
  resetUserForm()
  userAdvancedPanels.value = []
  userDialogMode.value = 'edit'
  userForm.username = user.username
  userForm.email = user.email || ''
  userForm.nickname = user.nickname || ''
  userForm.department_full_path = user.department_full_path || ''
  userForm.leader_username = user.leader_username || ''
  userDialogVisible.value = true
}

function openPasswordDialog(user: UserInfo) {
  selectedUsername.value = user.username
  passwordForm.password = ''
  passwordDialogVisible.value = true
}

function validateUserForm() {
  if (!/^[a-z][a-z0-9_]{2,31}$/.test(userForm.username)) {
    ElMessage.warning(t('systemUser.usernameInvalid'))
    return false
  }
  if (userDialogMode.value === 'create' && userForm.password.length < 6) {
    ElMessage.warning(t('systemUser.passwordInvalid'))
    return false
  }
  return true
}

async function saveUser() {
  if (!validateUserForm()) {
    return
  }
  savingUser.value = true
  try {
    if (userDialogMode.value === 'create') {
      await createSystemUser({
        username: userForm.username.trim(),
        password: userForm.password,
        email: userForm.email.trim() || undefined,
        nickname: userForm.nickname.trim() || undefined,
        department_full_path: userForm.department_full_path.trim() || undefined,
        leader_username: userForm.leader_username.trim() || undefined,
        status: userForm.status,
      })
      ElMessage.success(t('systemUser.created'))
    } else {
      await updateSystemUser(userForm.username, {
        email: userForm.email.trim(),
        nickname: userForm.nickname.trim(),
        department_full_path: userForm.department_full_path.trim(),
        leader_username: userForm.leader_username.trim(),
      })
      ElMessage.success(t('systemUser.updated'))
    }
    userDialogVisible.value = false
    await loadUsers()
  } catch (error: any) {
    ElMessage.error(error?.message || t('systemUser.saveFailed'))
  } finally {
    savingUser.value = false
  }
}

async function resetPassword() {
  if (passwordForm.password.length < 6) {
    ElMessage.warning(t('systemUser.passwordInvalid'))
    return
  }
  resettingPassword.value = true
  try {
    await resetSystemUserPassword(selectedUsername.value, passwordForm.password)
    ElMessage.success(t('systemUser.passwordReset'))
    passwordDialogVisible.value = false
  } catch (error: any) {
    ElMessage.error(error?.message || t('systemUser.passwordResetFailed'))
  } finally {
    resettingPassword.value = false
  }
}

async function handleStatusCommand(user: UserInfo, status: string) {
  try {
    const { value: reason } = await ElMessageBox.prompt(
      t('systemUser.statusConfirm', { username: user.username, status: statusLabel(status) }),
      t('systemUser.statusAction'),
      {
        type: status === 'disabled' ? 'warning' : 'info',
        inputPlaceholder: t('userImport.reason'),
        inputValidator: (value: string) => !!value?.trim() && value.trim().length <= 500 || t('userImport.reason'),
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
      }
    )
    await updateSystemUserStatus(user.username, status as 'active' | 'pending' | 'disabled', reason.trim())
    ElMessage.success(t('systemUser.statusUpdated'))
    await loadUsers()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error?.message || t('systemUser.statusUpdateFailed'))
    }
  }
}

async function handleUserActionCommand(user: UserInfo, command: string) {
  if (command === 'reset-password') {
    openPasswordDialog(user)
    return
  }
  if (command === 'active' || command === 'pending' || command === 'disabled') {
    await handleStatusCommand(user, command)
  }
}

function isStatusActionDisabled(user: UserInfo, status: string) {
  return user.username === 'system' || (user.status === status && status === 'active')
}

function normalizeUserCodeInput(value: string | number) {
  userForm.username = String(value ?? '')
    .toLowerCase()
    .replace(/[-.\s]+/g, '_')
    .replace(/[^a-z0-9_]/g, '')
    .replace(/_+/g, '_')
    .replace(/^[^a-z]+/g, '')
}

function statusLabel(status: string) {
  if (status === 'active') {
    return t('systemUser.statusActive')
  }
  if (status === 'disabled') {
    return t('systemUser.statusDisabled')
  }
  if (status === 'pending') {
    return t('systemUser.statusPending')
  }
  return status || '-'
}

function statusTagType(status: string) {
  if (status === 'active') {
    return 'success'
  }
  if (status === 'disabled') {
    return 'danger'
  }
  return 'warning'
}

function formatDateTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value || '-'
  }
  return date.toLocaleString()
}

onMounted(loadUsers)
</script>

<style scoped>
.system-user-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.user-toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-form {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
}

.filter-select {
  width: 128px;
}

.user-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.summary-item {
  min-width: 0;
  padding: 14px 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
}

.summary-value {
  display: block;
  font-size: 22px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}

.summary-label {
  display: block;
  margin-top: 4px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.user-table {
  width: 100%;
}

:deep(.operation-column .cell) {
  display: flex;
  justify-content: flex-end;
}

.operation-cell {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 2px;
}

.operation-icon-button {
  width: 28px;
  height: 28px;
  padding: 0;
  border-radius: 6px;
}

.operation-icon-button--more {
  color: var(--el-text-color-secondary);
}

.operation-menu-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 108px;
}

.operation-menu-item--danger {
  color: var(--el-color-danger);
}

.user-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.user-cell-main {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.username {
  color: var(--el-text-color-primary);
  font-weight: 700;
  line-height: 1.3;
}

.nickname {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.4;
}

.pagination-row {
  display: flex;
  justify-content: flex-end;
}

.user-form :deep(.el-select) {
  width: 100%;
}

.user-advanced-settings {
  margin-top: 4px;
  border-top: 1px solid var(--el-border-color-lighter);
}

@media (max-width: 768px) {
  .user-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .user-summary {
    grid-template-columns: 1fr;
  }

  .pagination-row {
    justify-content: flex-start;
    overflow-x: auto;
  }
}
</style>
