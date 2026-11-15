import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'

// A separate CI tools directory supplies Playwright; Go/product dependencies stay unchanged.
const require = createRequire(import.meta.url)
const { chromium } = require(process.env.QUALITY_PLAYWRIGHT_MODULE || resolve('scripts/reference-tools/node_modules/playwright'))
const prototype = resolve('docs/phase5/prototypes/s1-form-designer.html')
const source = await readFile(prototype, 'utf8')
const sortable = await readFile(resolve('docs/phase5/prototypes/vendor-sortable.min.js'))
assert(process.argv.slice(2).every((arg) => arg === '--self-test'), 'unknown prototype test argument')
const selfTest = process.argv.includes('--self-test')
const output = resolve(process.env.QUALITY_ARTIFACT_DIR || '.codex/audits/tiered-gates-20261116', `prototype-${Date.now()}`)
await mkdir(output, { recursive: true })
const browser = await chromium.launch()
const results = []

async function withPrototype(html, task) {
  const server = createServer((request, response) => {
    if (request.url === '/') {
      response.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }).end(html)
    } else if (request.url === '/vendor-sortable.min.js') {
      response.writeHead(200, { 'content-type': 'application/javascript' }).end(sortable)
    } else response.writeHead(404).end()
  })
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const context = await browser.newContext({ viewport: { width: 1366, height: 768 }, serviceWorkers: 'block' })
  const errors = [], external = []
  await context.route('**/*', async (route) => {
    if (new URL(route.request().url()).origin !== origin) {
      external.push(route.request().url())
      await route.abort()
    } else await route.continue()
  })
  await context.routeWebSocket('**/*', (socket) => { external.push(socket.url()); socket.close() })
  const page = await context.newPage()
  page.on('pageerror', (error) => errors.push(error.message))
  page.setDefaultTimeout(2000)
  try {
    await page.goto(origin, { waitUntil: 'networkidle' })
    await task(page)
    assert.deepEqual(external, [], 'prototype external network access')
    assert.deepEqual(errors, [], 'prototype script errors')
  } finally {
    await context.close()
    await new Promise((done) => server.close(done))
  }
}

async function schema(page) {
  await page.locator('#btn-schema').click()
  const value = JSON.parse(await page.locator('pre.schema').innerText())
  await page.locator('#btn-preview').click()
  await page.locator('#btn-preview').click()
  return value
}

async function importJSON(page, value) {
  page.once('dialog', (dialog) => dialog.accept(value))
  await page.locator('#btn-import').click()
}

async function requiredSurvivesExport(page) {
  await page.locator('[data-type="input"]').click()
  await page.locator('[data-p="required"]').check()
  assert.equal((await schema(page))[0].required, true, 'required serialization lost')
}

async function duplicateKeyIsRejected(page) {
  await page.locator('[data-type="input"]').click()
  await page.locator('[data-p="field_key"]').fill('synthetic_key')
  await page.locator('[data-type="input"]').click()
  const original = await page.locator('[data-p="field_key"]').inputValue()
  await page.locator('[data-p="field_key"]').fill('synthetic_key')
  assert.match(await page.locator('#key-err').innerText(), /非法或重复/, 'duplicate key rejection lost')
  assert.equal((await schema(page))[1].field_key, original, 'duplicate key modified stored value')
}

async function importRoundTrip(page) {
  const expected = [{ field_key: 'synthetic_key', field_label: '合成字段', field_type: 'input', required: true, sort_order: 1,
    field_meta: { validation: { regex: '^[A-Z]+$' }, formula: 'price * qty' } }]
  await importJSON(page, JSON.stringify(expected))
  assert.deepEqual(await schema(page), expected, 'JSON round trip lost data')
}

async function invalidJSONIsRejected(page) {
  await importJSON(page, '{broken}')
  assert.match(await page.locator('#toast').innerText(), /导入失败/, 'invalid JSON rejection lost')
}

async function fullSuite(page) {
  assert.equal(await page.locator('#palette .pal-item').count(), 8, 'eight field types must exist')
  assert.equal(await page.locator('.field-item').count(), 0)
  await duplicateKeyIsRejected(page)
  await page.locator('#btn-demo').click()
  assert.equal(await page.locator('.field-item').count(), 4, 'demo must load four fields')
  await page.locator('.field-item').first().click()
  await page.locator('[data-p="field_label"]').fill('合成字段 <安全示例>')
  await page.locator('[data-p="field_key"]').fill('synthetic_ip')
  await page.locator('[data-p="regex"]').fill('^[A-Z]+$')
  await page.locator('[data-p="formula"]').fill('price * qty')
  await page.locator('[data-p="regex"]').fill('[')
  assert.match(await page.locator('#regex-err').innerText(), /正则语法错误/, 'invalid regex must show feedback')
  await page.locator('.field-item').nth(1).click()
  await page.locator('[data-opt="0"] [data-o="label"]').fill('一级')
  await page.locator('[data-opt="0"] [data-o="value"]').fill('one')
  await page.locator('[data-act="add-opt"]').click()
  await page.locator('[data-opt="3"] [data-o="label"]').fill('四级')
  await page.locator('[data-opt="3"] [data-o="value"]').fill('four')
  await page.locator('[data-delopt="1"]').click()
  assert.equal(await page.locator('.opt-row').count(), 3)
  await page.screenshot({ path: resolve(output, '1366-design.png'), fullPage: true })
  const expected = await schema(page)
  assert.equal(expected[0].field_label, '合成字段 <安全示例>')
  assert.equal(expected[0].field_key, 'synthetic_ip')
  assert.equal(expected[0].required, true)
  assert.equal(expected[0].field_meta.validation.regex, '^[A-Z]+$', 'invalid regex must preserve previous stored regex')
  assert.equal(expected[0].field_meta.formula, 'price * qty')
  assert.deepEqual(expected[1].field_options.map((option) => option.value), ['one', 'low', 'four'])
  assert.deepEqual(expected.map((field) => field.sort_order), [1, 2, 3, 4])
  await importJSON(page, JSON.stringify(expected))
  assert.deepEqual(await schema(page), expected, 'edited JSON round trip lost data')
  await invalidJSONIsRejected(page)
  assert.deepEqual(await schema(page), expected, 'invalid JSON changed the previous fields')
  await page.locator('#btn-preview').click()
  assert.match(await page.locator('#panel').innerText(), /预览态不可编辑/)
  assert.equal(await page.locator('#canvas input[type="text"]').count(), 1)
  assert.equal(await page.locator('#canvas select option').count(), 3)
  assert.equal(await page.locator('#canvas textarea').count(), 1)
  assert.match(await page.locator('#canvas .tips-box').innerText(), /本节仅安全组填写/)
  await page.screenshot({ path: resolve(output, '1366-preview.png'), fullPage: true })
  await page.locator('#btn-preview').click()
  await page.locator('[data-del="0"]').click()
  const afterDelete = await schema(page)
  assert.equal(afterDelete.length, 3)
  assert.deepEqual(afterDelete.map((field) => field.sort_order), [1, 2, 3])
  assert(!afterDelete.some((field) => field.field_key === 'synthetic_ip'))
}

try {
  if (selfTest) {
    const cases = [
      ['required-serialization', 'required: !!f.required, sort_order: i + 1,', 'required: false, sort_order: i + 1,', requiredSurvivesExport, 'required serialization lost'],
      ['duplicate-key', '&& fields.every((x, k) => k === selected || x.field_key !== val)', '&& true', duplicateKeyIsRejected, 'duplicate key rejection lost'],
      ['json-round-trip', "field_key: x.field_key || '', field_label: x.field_label || '', field_type: x.field_type,", "field_key: '', field_label: x.field_label || '', field_type: x.field_type,", importRoundTrip, 'JSON round trip lost data'],
      ['invalid-json', "toast('导入失败：' + e.message)", "toast('导入已成功')", invalidJSONIsRejected, 'invalid JSON rejection lost'],
      ['external-network', '</body>', '<script>fetch("https://example.invalid/prototype-mutation").catch(()=>{})</script></body>', async () => {}, 'prototype external network access'],
    ]
    for (const [name, before, after, check, expectedFailure] of cases) {
      assert.equal(source.split(before).length, 2, `${name}: mutation target must occur once`)
      await withPrototype(source, check)
      let rejected = false
      try { await withPrototype(source.replace(before, after), check) }
      catch (error) {
        assert(error.message.includes(expectedFailure), `${name}: unrelated failure: ${error.message}`)
        rejected = true
        results.push({ name, result: 'rejected', reason: expectedFailure })
      }
      assert(rejected, `${name}: target defect was accepted`)
      process.stdout.write(`PASS mutation rejected: ${name}\n`)
    }
  } else {
    await withPrototype(source, fullSuite)
    results.push({ name: 'form-designer-offline-tasks', viewport: '1366x768', result: 'pass' })
    process.stdout.write('PASS form designer offline fields/options/schema/import/preview/delete tasks\n')
  }
  await writeFile(resolve(output, 'result.json'), JSON.stringify({ prototype, selfTest, results }, null, 2) + '\n')
  process.stdout.write(`Artifacts: ${output}\n`)
} finally {
  await browser.close()
}
