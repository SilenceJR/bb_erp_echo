import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import {formatShelfLocationCode, generatedShelfCodes, missingShelfCodes, parseShelfLocationCode, PALLET_LOCATION_CODE} from '../src/components/pages/module/moldLocation.ts'

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')

test('位置编码保留 A1-1 格式并支持默认范围与扩展去重', () => {
  assert.deepEqual(parseShelfLocationCode('a7-4'), {zone: 'A', row: 7, column: 4})
  assert.equal(formatShelfLocationCode(' e ', 2, 2), 'E2-2')
  assert.equal(generatedShelfCodes('A', 7, 4).length, 28)
  assert.equal(generatedShelfCodes('B', 6, 4).length, 24)
  assert.deepEqual(missingShelfCodes([{id: 1, code: 'E1-1'}, {id: 2, code: PALLET_LOCATION_CODE}], 'E', 2, 2), ['E1-2', 'E2-1', 'E2-2'])
})

test('不规则排列和停用位置仍按已有编码去重', () => {
  const locations = [
    {id: 1, code: 'E1-1', status: 'active'},
    {id: 2, code: 'E1-2', status: 'disabled'},
    {id: 3, code: 'E2-1', status: 'active'},
  ]
  assert.deepEqual(missingShelfCodes(locations, 'E', 2, 2), ['E2-2'])
  const picker = read('../src/components/pages/module/MoldLocationPicker.vue')
  assert.match(picker, /function onRowChange()/)
  assert.match(picker, /if \(!columns\.value\.includes\(column\.value \|\| -1\)\) column\.value = columns\.value\[0\]/)
  assert.match(picker, /const isLegacy = computed\(\(\) => Boolean\(selectedLocation\.value/)
})

test('模具页面使用位置选择器、批量位置接口和中央详情载体', () => {
  const mold = read('../src/components/pages/module/MoldModuleContent.vue')
  const picker = read('../src/components/pages/module/MoldLocationPicker.vue')
  assert.match(mold, /<MoldLocationPicker v-model="draft\.location_id"/)
  assert.match(mold, /<MoldLocationPicker v-model="bulkLocationID"/)
  assert.match(mold, /<MoldLocationPicker v-model="filters\.locationID"/)
  assert.match(mold, /'\/api\/v1\/mold-locations\/bulk'/)
  assert.match(mold, /body: \{zone, rows: bulkLocation\.rows, columns: bulkLocation\.columns\}/)
  assert.doesNotMatch(mold, /<ResponsiveDetailCarrier[\s\S]*v-model="detailVisible"/)
  assert.match(mold, /class="mold-detail-page"/)
  assert.match(picker, /<el-radio-button value="shelf">货架<\/el-radio-button>/)
  assert.match(picker, /<el-radio-button value="pallet">卡板<\/el-radio-button>/)
  assert.match(picker, /历史位置/)
  assert.match(mold, /:data-mold-id="row\.id"/)
  assert.match(mold, /document\.querySelector<HTMLElement>\(`\[data-mold-id="\$\{returnFocus\.id\}"\]`\)/)
  assert.match(mold, /\.mold-create-button/)
  assert.doesNotMatch(mold, /detailReturnFocus = ref<HTMLElement>/)
})

test('模具 DWG 与 ZIP 拖放拒绝多文件并处理原生拖放错误和离开', () => {
  const mold = read('../src/components/pages/module/MoldModuleContent.vue')
  assert.match(mold, /请一次拖入一个 DWG 或 FDWG 文件/)
  assert.match(mold, /请一次拖入一个 ZIP 资料包/)
  assert.match(mold, /const totalFiles = detail\.paths\.length \+ detail\.files\.length/)
  assert.match(mold, /if \(detail\.phase === 'leave'\) return/)
  assert.match(mold, /当前账号没有图纸上传权限/)
})

test('模具详情使用紧凑工具栏、容器布局和图库专用变体', () => {
  const mold = read('../src/components/pages/module/MoldModuleContent.vue')
  const gallery = read('../src/components/ImageGallery.vue')
  const galleryStyles = read('../src/styles/feature-patterns.css')
  assert.match(mold, /container-name: mold-detail/)
  assert.match(mold, /class="mold-detail-assets"/)
  assert.match(mold, /variant="mold-detail"/)
  assert.match(mold, /mold-form-item--wide/)
  assert.match(mold, /dirtyGuardRegistry\.confirmLeave\('dialog-close'\)/)
  assert.match(mold, /@container mold-detail \(min-width: 1280px\)/)
  assert.match(mold, /minmax\(0, 1\.35fr\).*minmax\(0, \.85fr\)/)
  assert.match(gallery, /variant === 'mold-detail'/)
  assert.match(gallery, /variant\?: 'default' \| 'mold-detail'/)
  assert.match(gallery, /image-gallery-product-layout/)
  assert.match(gallery, /chooseMainImage\(item\.id\)/)
  assert.match(gallery, /slice\(thumbnailPage\.value \* 3/)
  assert.match(galleryStyles, /\.image-gallery--mold-detail \{ margin: 0; border-top: 0; padding-top: 0; \}/)
  assert.match(galleryStyles, /\.image-gallery-product-layout \{[^}]*clamp\(220px, 24vw, 320px\)/)
  assert.match(galleryStyles, /\.image-gallery--mold-detail\.image-gallery--supplement \.image-gallery-grid \{[^}]*minmax\(144px, 1fr\)/)
})
