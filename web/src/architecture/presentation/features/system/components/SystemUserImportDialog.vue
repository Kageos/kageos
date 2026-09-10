<template>
  <el-dialog :model-value="modelValue" :title="t('userImport.title')" width="min(920px, 95vw)" :close-on-click-modal="!busy" :show-close="!busy" :close-on-press-escape="!busy" @update:model-value="close">
    <el-alert :title="t('userImport.hint')" type="info" :closable="false" show-icon />
    <div class="import-toolbar">
      <el-button :disabled="busy" @click="downloadTemplate">{{ t('userImport.template') }}</el-button>
      <label class="import-file">{{ t('userImport.file') }} <input type="file" accept=".xlsx" :disabled="busy" @change="readFile" /></label>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-table :data="results" max-height="360" :empty-text="t('userImport.empty')">
      <el-table-column prop="row" :label="t('userImport.row')" width="80" />
      <el-table-column prop="username" :label="t('systemUser.username')" min-width="150" />
      <el-table-column :label="t('systemUser.status')" width="140"><template #default="{ row }"><el-tag :type="row.status === 'failed' ? 'danger' : 'success'">{{ t(`userImport.${row.status}`) }}</el-tag></template></el-table-column>
      <el-table-column prop="message" :label="t('userImport.result')" min-width="230" />
    </el-table>
    <template #footer>
      <el-button :disabled="busy" @click="close(false)">{{ t('common.close') }}</el-button>
      <el-button :disabled="busy || !rows.length" @click="validate">{{ t('userImport.validate') }}</el-button>
      <el-button type="primary" :loading="busy" :disabled="!readyCount || committed" @click="create">{{ t('userImport.create', { count: readyCount }) }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { readUserImportWorkbook, userImportColumns } from './utils/userImportWorkbook'
import { useI18n } from 'vue-i18n'
import { importSystemUsers, type SystemCreateUserReq, type SystemImportUserResult } from '@/architecture/presentation/context/api/user'
const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; created: [] }>()
const { t } = useI18n()
const rows = ref<SystemCreateUserReq[]>([])
const results = ref<SystemImportUserResult[]>([])
const busy = ref(false)
const committed = ref(false)
const error = ref('')
const readyCount = computed(() => results.value.filter(row => row.status === 'ready').length)
const columns = userImportColumns
function close(value: boolean) {
  if (busy.value) return
  if (!value) { rows.value = []; results.value = []; error.value = ''; committed.value = false }
  emit('update:modelValue', value)
}
async function downloadTemplate() {
  busy.value = true
  try {
    const { Workbook } = await import('exceljs')
    const workbook = new Workbook()
    const sheet = workbook.addWorksheet('users')
    sheet.addRow(columns)
    sheet.columns.forEach(column => { column.width = 26; column.numFmt = '@' })
    const blob = new Blob([await workbook.xlsx.writeBuffer()], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a'); link.href = url; link.download = 'users-template.xlsx'; link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch { error.value = t('userImport.readError') } finally { busy.value = false }
}
async function readFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  rows.value = []; results.value = []; error.value = ''; committed.value = false
  busy.value = true
  try {
    if (!file.name.toLowerCase().endsWith('.xlsx') || file.size > 1024 * 1024) throw new Error(t('userImport.readError'))
    rows.value = await readUserImportWorkbook(await file.arrayBuffer())
  } catch { rows.value = []; error.value = t('userImport.readError') }
  finally { busy.value = false; input.value = '' }
  if (rows.value.length) await validate()
}
async function validate() {
  busy.value = true; error.value = ''; committed.value = false; results.value = []
  try { results.value = (await importSystemUsers(rows.value, true)).rows }
  catch (cause) { error.value = cause instanceof Error ? cause.message : t('userImport.readError') }
  finally { busy.value = false }
}
async function create() {
  if (!props.modelValue || busy.value || committed.value || !readyCount.value) return
  busy.value = true; error.value = ''; committed.value = true
  try {
    results.value = (await importSystemUsers(rows.value, false)).rows
    emit('created')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : t('userImport.uncertain') }
  finally { busy.value = false }
}
</script>
<style scoped>
.import-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; margin: 20px 0; }
.import-file { display: flex; flex-wrap: wrap; gap: 10px; color: var(--el-text-color-regular); }
</style>
