/**
 * SimVerse 模拟世界相关类型定义
 */

/** NPC 实体 */
export interface NPC {
  id: string
  name: string
  status: 'active' | 'inactive' | 'sleeping'
  position?: { x: number; y: number; z: number }
  dialogue?: string
}

/** 编年史事件 */
export interface Chronicle {
  id: string
  title: string
  content: string
  tick: number
  timestamp: number
}

/** 世界状态 */
export interface WorldState {
  tick: number
  era: string
  npcCount: number
  isActive: boolean
  config?: {
    performance_tier: 'low' | 'mid' | 'high'
  }
}
