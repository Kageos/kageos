import { describe, expect, it } from 'vitest'
import { Workbook } from 'exceljs'
import { readUserImportWorkbook, userImportColumns } from './userImportWorkbook'
async function workbookBuffer(row: unknown[]) {
  const workbook = new Workbook()
  const sheet = workbook.addWorksheet('users')
  sheet.addRow(userImportColumns)
  sheet.addRow(row)
  const buffer = await workbook.xlsx.writeBuffer()
  return new Uint8Array(buffer).buffer
}
describe('user import workbook', () => {
  it('preserves passwords verbatim and creates disabled accounts', async () => {
    const rows = await readUserImportWorkbook(await workbookBuffer(['alice', '00123456', 'Alice', '', '']))
    expect(rows).toEqual([{ username: 'alice', password: '00123456', nickname: 'Alice', email: '', department_full_path: '', status: 'disabled' }])
  })
  it('rejects formulas rather than trusting cached formula results', async () => {
    await expect(readUserImportWorkbook(await workbookBuffer(['alice', { formula: '1+1', result: '12345678' }]))).rejects.toThrow('text-required')
  })
  it('rejects numeric passwords to avoid silently changing credentials', async () => {
    await expect(readUserImportWorkbook(await workbookBuffer(['alice', 12345678]))).rejects.toThrow('text-required')
  })
  it('rejects empty and oversized uploads', async () => {
    await expect(readUserImportWorkbook(new ArrayBuffer(1024 * 1024 + 1))).rejects.toThrow('file-size')
  })
})
