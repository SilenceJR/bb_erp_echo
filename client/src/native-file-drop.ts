import {getCurrentWebview} from '@tauri-apps/api/webview'

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
  if (!position) return activeTarget
  const ratio = window.devicePixelRatio || 1
  const element = document.elementFromPoint(position.x / ratio, position.y / ratio)
  return element instanceof HTMLElement ? element.closest<HTMLElement>('[data-file-drop-target]') : null
}

function emit(target: HTMLElement | null, detail: NativeFileDragDetail) {
  target?.dispatchEvent(new CustomEvent<NativeFileDragDetail>(eventName, {bubbles: true, detail}))
}

function installNativeFileDrop() {
  void getCurrentWebview().onDragDropEvent(async ({payload}: {payload: NativeDragPayload}) => {
    if (payload.type === 'leave') {
      emit(activeTarget, {phase: 'leave', paths: [], files: []})
      activeTarget = null
      return
    }

    const target = targetAt(payload.position)
    if (payload.type === 'enter' || payload.type === 'over') {
      if (target) activeTarget = target
      emit(target, {phase: payload.type, paths: [], files: []})
      return
    }

    activeTarget = null
    if (!target) return
    emit(target, {phase: 'drop', paths: payload.paths || [], files: []})
  })
}

installNativeFileDrop()
