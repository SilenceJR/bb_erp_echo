import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'
import './design-system.css'
// MessageBox is created through a service API, so its component stylesheet is not
// guaranteed to be included by the component auto-import resolver.
import 'element-plus/theme-chalk/el-message-box.css'
import {applyAppearance} from './platform/appearance'

// 应用入口：挂载博邦光电 ERP Web 管理端。
// 在 Vue 和 Element Plus 挂载前恢复外观，避免客户端启动时出现亮色闪烁。
applyAppearance(document.documentElement, window.localStorage)
createApp(App).mount('#app')
