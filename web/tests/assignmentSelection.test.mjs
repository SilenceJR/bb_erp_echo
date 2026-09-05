import assert from 'node:assert/strict'
import test from 'node:test'
import {computed, nextTick, reactive, ref} from 'vue'

test('异步加入权限缓存后就绪状态会响应式更新，批量复选不被永久禁用', async () => {
  const assignmentOptionsCache = reactive({})
  const assignmentConfig = ref({optionKey: 'permissions'})
  const assignmentOptionsLoading = ref(true)
  const assignmentOptionsError = ref('')
  const assignmentOptionsReady = computed(() => {
    const config = assignmentConfig.value
    const cachedOptions = config ? assignmentOptionsCache[config.optionKey] : undefined
    return Boolean(config && Array.isArray(cachedOptions) && !assignmentOptionsLoading.value && !assignmentOptionsError.value)
  })

  assert.equal(assignmentOptionsReady.value, false)
  assignmentOptionsCache.permissions = [{id: 1, code: 'customers:read'}]
  assignmentOptionsLoading.value = false
  await nextTick()
  assert.equal(assignmentOptionsReady.value, true)
})

test('全选计算使用完整可授予集合，不受当前搜索结果影响', () => {
  const options = [
    {id: 1, name: '客户查看', code: 'customers:read', disabled: false},
    {id: 2, name: '客户维护', code: 'customers:write', disabled: false},
    {id: 3, name: '库存维护', code: 'inventory:write', disabled: true},
  ]
  const selected = new Set()
  const query = '客户'
  const assignable = options.filter((option) => !option.disabled)
  const visible = options.filter((option) => option.name.includes(query))
  for (const option of assignable) selected.add(option.id)

  assert.equal(visible.length, 2)
  assert.deepEqual([...selected].sort(), [1, 2])
  assert.equal(assignable.length, 2)
})
