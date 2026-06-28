import type { Memory, MemoryList } from '../api/client'
import { memoryCategories, type MemoryCategory, type MemoryViewModel } from './dashboard'

export type KnowledgeDiscoveryCard = MemoryViewModel & {
  readonly highlights: readonly string[]
}

export type KnowledgeDiscoveryData = {
  readonly items: readonly KnowledgeDiscoveryCard[]
  readonly total: number
  readonly limit: number
  readonly offset: number
  readonly previousOffset: number | null
  readonly nextOffset: number | null
}

export function memoryListToDiscoveryData(page: MemoryList): KnowledgeDiscoveryData {
  return memoryPageToDiscoveryData(page.memories, page)
}

function memoryPageToDiscoveryData(memories: readonly Memory[], page: { total: number; limit: number; offset: number }): KnowledgeDiscoveryData {
  return {
    items: memories.map(memoryToDiscoveryCard),
    total: page.total,
    limit: page.limit,
    offset: page.offset,
    previousOffset: previousOffset(page),
    nextOffset: nextOffset(page)
  }
}

function memoryToDiscoveryCard(memory: Memory): KnowledgeDiscoveryCard {
  return {
    id: memory.id,
    title: memory.title,
    content: memory.content,
    category: memoryCategory(memory.category),
    projectId: memory.project,
    authorId: memory.created_by,
    authorLabel: memory.created_by,
    tags: memory.tags,
    savedAt: memory.created_at,
    savedAtLabel: dateLabel(memory.created_at),
    highlights: []
  }
}

function previousOffset(page: { limit: number; offset: number }): number | null {
  return page.offset <= 0 ? null : Math.max(0, page.offset - page.limit)
}

function nextOffset(page: { total: number; limit: number; offset: number }): number | null {
  const next = page.offset + page.limit
  return next >= page.total ? null : next
}

function memoryCategory(value: string): MemoryCategory {
  return memoryCategories.includes(value as MemoryCategory) ? value as MemoryCategory : 'discovery'
}

function dateLabel(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('en-GB', { day: '2-digit', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(date)
}
