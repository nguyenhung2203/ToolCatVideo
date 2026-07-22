import {createApp} from 'vue'
import App from './App.vue'
import './ui-system/foundations'
import './style.css'

// Khóa menu chuột phải mặc định của trình duyệt
window.addEventListener('contextmenu', (e) => e.preventDefault())

createApp(App).mount('#app')
