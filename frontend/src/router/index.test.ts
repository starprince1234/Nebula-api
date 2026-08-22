import { describe, expect, it } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'
import router, { routes } from './index'

function namedRouteNames(records: RouteRecordRaw[]): string[] {
  return records.flatMap(route => [
    ...(route.name ? [String(route.name)] : []),
    ...('children' in route && route.children ? namedRouteNames(route.children) : []),
  ])
}

describe('console router', () => {
  it('keeps every named route unique and resolves both project management pages', () => {
    const names = namedRouteNames(routes)

    expect(new Set(names).size).toBe(names.length)
    expect(router.resolve('/mentor/projects').matched.at(-1)?.name).toBe('mentor-projects')
    expect(router.resolve('/teacher/projects').matched.at(-1)?.name).toBe('teacher-projects')
  })

  it('uses route metadata for localized workspace titles', () => {
    expect(router.resolve('/mentor/projects').meta.title).toBe('项目管理')
    expect(router.resolve('/mentor/reviews').meta.title).toBe('审核密钥')
    expect(router.resolve('/teacher/projects').meta.title).toBe('项目管理')
  })
})
