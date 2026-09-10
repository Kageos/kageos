import type { SystemCreateUserReq } from '@/architecture/presentation/context/api/user'
export const userImportColumns = ['username', 'password', 'nickname', 'email', 'department_full_path']

export async function readUserImportWorkbook(buffer: ArrayBuffer): Promise<SystemCreateUserReq[]> {
  if (buffer.byteLength > 1024 * 1024) throw new Error('file-size')
  const { Workbook } = await import('exceljs')
  const workbook = new Workbook()
  await workbook.xlsx.load(buffer)
  const sheet = workbook.worksheets[0]
  if (!sheet || sheet.rowCount < 2 || sheet.rowCount > 101 || sheet.columnCount > userImportColumns.length) throw new Error('dimensions')
  userImportColumns.forEach((column, index) => {
    if (sheet.getRow(1).getCell(index + 1).value !== column) throw new Error('headers')
  })
  const rows: SystemCreateUserReq[] = []
  for (let index = 2; index <= sheet.rowCount; index++) {
    const values = userImportColumns.map((_, column) => {
      const value = sheet.getRow(index).getCell(column + 1).value
      if (value === null || value === undefined) return ''
      // Passwords/identifiers must be text so Excel cannot silently drop zeros.
      if (typeof value !== 'string') throw new Error('text-required')
      return value
    })
    rows.push({ username: values[0] || '', password: values[1] || '', nickname: values[2] || '', email: values[3] || '', department_full_path: values[4] || '', status: 'disabled' })
  }
  return rows
}
