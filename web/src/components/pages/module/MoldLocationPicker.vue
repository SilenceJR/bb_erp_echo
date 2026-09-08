<template>
  <div class="mold-location-picker" :class="{'is-compact': compact}">
    <el-radio-group v-model="kind" :disabled="disabled" :aria-label="label || '存放方式'" @change="changeKind">
      <el-radio-button value="shelf">货架</el-radio-button>
      <el-radio-button value="pallet">卡板</el-radio-button>
    </el-radio-group>

    <div v-if="isLegacy" class="mold-location-picker__legacy">
      <span>历史位置</span>
      <el-tag effect="plain">{{ selectedLocation?.code }}</el-tag>
      <el-button link type="primary" :disabled="disabled" @click="adoptShelf">改用货架位置</el-button>
    </div>

    <div v-else-if="kind === 'shelf'" class="mold-location-picker__shelf">
      <el-select v-model="zone" :disabled="disabled" :size="size" placeholder="区" :aria-label="`${label || '模具'}位置区`" @change="onZoneChange">
        <el-option v-for="value in zones" :key="value" :label="value" :value="value" />
      </el-select>
      <el-select v-model="row" :disabled="disabled || !zone" :size="size" placeholder="排" :aria-label="`${label || '模具'}位置排`" @change="onRowChange">
        <el-option v-for="value in rows" :key="value" :label="String(value)" :value="value" />
      </el-select>
      <el-select v-model="column" :disabled="disabled || !zone || !row" :size="size" placeholder="列" :aria-label="`${label || '模具'}位置列`" @change="emitShelfSelection">
        <el-option v-for="value in columns" :key="value" :label="String(value)" :value="value" />
      </el-select>
    </div>

    <span v-if="kind === 'pallet'" class="mold-location-picker__hint" :aria-hidden="compact ? 'true' : undefined">独立放置，不占用货架位置</span>
    <el-button v-if="clearable && modelValue !== undefined && modelValue !== null" class="mold-location-picker__clear" link type="info" :disabled="disabled" @click="clearSelection">清除位置</el-button>
  </div>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {ElMessage} from 'element-plus'
import {formatShelfLocationCode, isPalletLocation, parseShelfLocationCode, PALLET_LOCATION_CODE, type MoldLocationOption} from './moldLocation'

const props = withDefaults(defineProps<{
  modelValue?: number
  locations: MoldLocationOption[]
  disabled?: boolean
  clearable?: boolean
  compact?: boolean
  size?: 'small' | 'default' | 'large'
  label?: string
}>(), {
  modelValue: undefined,
  disabled: false,
  clearable: false,
  compact: false,
  size: 'default',
  label: '模具',
})

const emit = defineEmits<{(event: 'update:modelValue', value: number | undefined): void}>()
const kind = ref<'shelf' | 'pallet'>('shelf')
const zone = ref('')
const row = ref<number>()
const column = ref<number>()
const selectionTouched = ref(false)

const selectedLocation = computed(() => props.locations.find((location) => location.id === props.modelValue))
const parsedLocation = computed(() => selectedLocation.value ? parseShelfLocationCode(selectedLocation.value.code) : null)
const isLegacy = computed(() => Boolean(selectedLocation.value && !isPalletLocation(selectedLocation.value) && !parsedLocation.value && !selectionTouched.value))
const shelfCandidates = computed(() => props.locations.filter((location) => Boolean(parseShelfLocationCode(location.code))))
const zones = computed(() => [...new Set(shelfCandidates.value.map((location) => parseShelfLocationCode(location.code)?.zone).filter((value): value is string => Boolean(value)))].sort())
const rows = computed(() => [...new Set(shelfCandidates.value.map((location) => parseShelfLocationCode(location.code)).filter((parts): parts is NonNullable<typeof parts> => parts?.zone === zone.value).map((parts) => parts.row))].sort((a, b) => a - b))
const columns = computed(() => [...new Set(shelfCandidates.value.map((location) => parseShelfLocationCode(location.code)).filter((parts): parts is NonNullable<typeof parts> => parts?.zone === zone.value && parts.row === row.value).map((parts) => parts.column))].sort((a, b) => a - b))

function updateFromModel() {
  selectionTouched.value = false
  const location = selectedLocation.value
  if (!location) {
    kind.value = 'shelf'
    zone.value = ''
    row.value = undefined
    column.value = undefined
    return
  }
  if (isPalletLocation(location) || location.code === PALLET_LOCATION_CODE) {
    kind.value = 'pallet'
    zone.value = ''
    row.value = undefined
    column.value = undefined
    return
  }
  const parsed = parseShelfLocationCode(location.code)
  kind.value = 'shelf'
  if (!parsed) {
    zone.value = ''
    row.value = undefined
    column.value = undefined
    return
  }
  zone.value = parsed.zone
  row.value = parsed.row
  column.value = parsed.column
}

watch(() => props.modelValue, updateFromModel, {immediate: true})
watch(() => props.locations, updateFromModel, {deep: true})

function changeKind(value: string | number | boolean | undefined) {
  selectionTouched.value = true
  if (value === 'pallet') {
    const pallet = props.locations.find((location) => isPalletLocation(location) || location.code === PALLET_LOCATION_CODE)
    if (!pallet) {
      kind.value = 'shelf'
      ElMessage.warning('卡板位置尚未初始化，请先刷新位置字典')
      return
    }
    emit('update:modelValue', pallet.id)
    return
  }
  kind.value = 'shelf'
  if (!zone.value) {
    zone.value = zones.value[0] || ''
    row.value = rows.value[0]
    column.value = columns.value[0]
  }
  emitShelfSelection()
}

function onZoneChange() {
  selectionTouched.value = true
  row.value = rows.value[0]
  column.value = columns.value[0]
  emitShelfSelection()
}

function onRowChange() {
  selectionTouched.value = true
  if (!columns.value.includes(column.value || -1)) column.value = columns.value[0]
  emitShelfSelection()
}

function emitShelfSelection() {
  selectionTouched.value = true
  if (!zone.value || row.value === undefined || column.value === undefined) {
    emit('update:modelValue', undefined)
    return
  }
  const code = formatShelfLocationCode(zone.value, row.value, column.value)
  const location = props.locations.find((candidate) => candidate.code.toUpperCase() === code)
  if (!location) {
    ElMessage.warning(`位置 ${code} 尚未创建，请先在位置管理中扩展该区`)
    return
  }
  emit('update:modelValue', location.id)
}

function adoptShelf() {
  selectionTouched.value = true
  kind.value = 'shelf'
  zone.value = zones.value[0] || ''
  row.value = rows.value[0]
  column.value = columns.value[0]
  emitShelfSelection()
}

function clearSelection() {
  selectionTouched.value = true
  kind.value = 'shelf'
  zone.value = ''
  row.value = undefined
  column.value = undefined
  emit('update:modelValue', undefined)
}
</script>

<style scoped>
.mold-location-picker { display: grid; min-width: 0; gap: var(--bb-space-2); }
.mold-location-picker__shelf { display: grid; grid-template-columns: minmax(0, 1fr) repeat(2, minmax(72px, 1fr)); gap: var(--bb-space-2); min-width: 0; }
.mold-location-picker__shelf :deep(.el-select) { min-width: 0; }
.mold-location-picker__legacy { display: flex; min-width: 0; align-items: center; flex-wrap: wrap; gap: var(--bb-space-2); color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.mold-location-picker__hint { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.mold-location-picker__clear { justify-self: start; padding: 0; }
.mold-location-picker.is-compact { display: flex; min-height: 40px; align-items: flex-start; flex-wrap: nowrap; gap: 4px; overflow: hidden; }
.mold-location-picker.is-compact :deep(.el-radio-group) { flex: 0 0 auto; min-height: 40px; }
.mold-location-picker.is-compact :deep(.el-radio-button__inner) { padding: 6px 9px; }
.mold-location-picker.is-compact .mold-location-picker__shelf,
.mold-location-picker.is-compact .mold-location-picker__hint { flex: 1 1 auto; min-width: 0; min-height: 40px; }
.mold-location-picker.is-compact .mold-location-picker__shelf { flex: 1 1 auto; grid-template-columns: repeat(3, minmax(44px, 1fr)); gap: 4px; min-width: 0; }
.mold-location-picker.is-compact .mold-location-picker__hint { display: block; font-size: 0; }
.mold-location-picker.is-compact .mold-location-picker__clear { flex: 0 0 auto; min-height: 40px; padding-right: 2px; padding-left: 2px; white-space: nowrap; }
@media (max-width: 760px) {
  .mold-location-picker__shelf { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
</style>
