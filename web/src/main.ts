import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'
import './design-system.css'
// MessageBox is created through a service API, so its component stylesheet is not
// guaranteed to be included by the component auto-import resolver.
import 'element-plus/theme-chalk/el-message-box.css'

// 应用入口：挂载博邦光电 ERP Web 管理端。
createApp(App).mount('#app')
