<template>
  <el-form id="temporary-product-form" class="temporary-product-form" label-position="top" :disabled="temporaryProductSubmitting" @submit.prevent="createTemporaryProduct">
    <FormPanelContent>
      <el-alert title="保存后将成为正式产品档案，初始库存为 0，不会生成入库记录。" type="info" :closable="false" show-icon />
      <FormSection title="产品资料" description="保存后将自动选入当前任务单。">
        <FormGrid columns="one">
          <el-form-item label="产品名称" required><el-input id="temporary-product-name" v-model.trim="temporaryProductForm.name" maxlength="100" show-word-limit autocomplete="off" /></el-form-item>
          <el-form-item label="产品编码" required><el-input v-model.trim="temporaryProductForm.code" maxlength="100" show-word-limit autocomplete="off" placeholder="请输入唯一编码" /></el-form-item>
          <el-form-item label="单位" required><el-input v-model.trim="temporaryProductForm.unit" maxlength="20" autocomplete="off" /></el-form-item>
          <el-form-item label="规格"><el-input v-model.trim="temporaryProductForm.spec" maxlength="200" show-word-limit autocomplete="off" placeholder="选填" /></el-form-item>
        </FormGrid>
      </FormSection>
      <FormSection title="操作记录" description="记录本次临时建档的实际操作人。">
        <OperatorSelect
          v-model="temporaryProductForm.operator_employee_id"
          :department="operatorDirectory.department.value"
          :employees="operatorDirectory.employees.value"
          :loading="operatorDirectory.loading.value"
          :unavailable-reason="operatorDirectory.unavailableReason.value"
          :retryable="operatorDirectory.retryable.value"
          @load="operatorDirectory.load"
          @retry="operatorDirectory.load(true)"
        />
      </FormSection>
      <el-alert v-if="temporaryProductError" :title="temporaryProductError" type="error" :closable="false" show-icon />
    </FormPanelContent>
  </el-form>
</template>

<script setup lang="ts">
import {useWorkorderContext} from '../../composables/workorderContext'
import OperatorSelect from '../ui/OperatorSelect.vue'
import FormPanelContent from '../ui/FormPanelContent.vue'
import FormSection from '../ui/FormSection.vue'
import FormGrid from '../ui/FormGrid.vue'

const {operatorDirectory, temporaryProductForm, temporaryProductSubmitting, temporaryProductError, createTemporaryProduct} = useWorkorderContext().product
</script>
