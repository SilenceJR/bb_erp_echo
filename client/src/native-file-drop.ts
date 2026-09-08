import {getCurrentWebview} from '@tauri-apps/api/webview'
import {ElMessage} from 'element-plus'

type NativeDragPhase = 'enter' | 'over' | 'leave' | 'drop'
type NativeDragPayload = {
  type: NativeDragPhase
  paths?: string[]
  position?: {x: number; y: number}
}

type NativeFileDragDetail = {
  phase: NativeDragPhase
  paths: string[]
  files: File[]
  error?: string
}

const eventName = 'bb-native-file-drag'
let activeTarget: HTMLElement | null = null

function targetAt(position?: {x: number; y: number}): HTMLElement | null {
  if (!position) return activeTarget?.isConnected ? activeTarget : null
  // Tauri gives physical pixels; DOM hit testing uses CSS pixels, including WebView zoom.
  const ratio = window.devicePixelRatio || 1
  const element = document.elementFromPoint(position.x / ratio, position.y / ratio)
  // SVG icons are Elements too, but are not HTMLElements.
  return element?.closest<HTMLElement>('[data-file-drop-target]') ?? null
}

function emit(target: HTMLElement | null, detail: NativeFileDragDetail) {
  target?.dispatchEvent(new CustomEvent<NativeFileDragDetail>(eventName, {bubbles: true, detail}))
}

function clearTarget() {
  emit(activeTarget, {phase: 'leave', paths: [], files: []})
  activeTarget = null
}

function handleNativeDrag({payload}: {payload: NativeDragPayload}) {
  if (payload.type === 'leave') {
    clearTarget()
    return
  }

  const target = targetAt(payload.position)
  if (target !== activeTarget) clearTarget()
  if (payload.type === 'enter' || payload.type === 'over') {
    activeTarget = target
    emit(target, {phase: payload.type, paths: payload.paths || [], files: []})
    return
  }

  clearTarget()
  if (!target) {
    ElMessage.warning('请将文件拖到图片、图纸或导入文件区域')
    return
  }
  emit(target, {phase: 'drop', paths: payload.paths || [], files: []})
}

async function installNativeFileDrop() {
  try {
    const unlisten = await getCurrentWebview().onDragDropEvent(handleNativeDrag)
    window.addEventListener('pagehide', () => { clearTarget(); unlisten() }, {once: true})
  } catch (error) {
    console.error('原生文件拖放监听失败', error)
    ElMessage.error({message: '文件拖放未能启用，请重新打开客户端；也可点击选择文件上传。', duration: 0, showClose: true})
  }
}

void installNativeFileDrop()
