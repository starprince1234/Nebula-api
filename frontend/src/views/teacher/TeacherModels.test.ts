import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TeacherModels from './TeacherModels.vue'
import { teacherAPI } from '../../api/teacher'

vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ replace: vi.fn() }) }))
vi.mock('../../api/teacher', () => ({ teacherAPI: { models: vi.fn(), providers: vi.fn(), model: vi.fn(), catalogCandidates: vi.fn(), createModel: vi.fn(), updateModel: vi.fn(), createBinding: vi.fn(), updateBinding: vi.fn(), probeModel: vi.fn() } }))

const AppDialogStub = { props: ['open', 'title'], emits: ['update:open'], template: '<div v-if="open" role="dialog" :aria-label="title"><h2>{{title}}</h2><slot/></div>' }
const mountView = () => mount(TeacherModels, { global: { stubs: { LoadingRegion: { template: '<div><slot/></div>' }, AppDialog: AppDialogStub, StatusBadge: true } } })

describe('TeacherModels', () => {
  beforeEach(() => {
    const model = { model_id: 'm1', display_name: 'M1', description: null, category: 'text' as const, capabilities: [], input_modalities: [], output_modalities: [], context_window: null, max_input_tokens: null, max_output_tokens: null, is_common: true, status: 'active' as const, route_ready: true, credit_multiplier: '1.000' }
    vi.mocked(teacherAPI.models).mockResolvedValue([model])
    vi.mocked(teacherAPI.model).mockResolvedValue({ model, bindings: [] })
    vi.mocked(teacherAPI.providers).mockResolvedValue([{ id: 'provider-1', name: 'Matrix', base_url: 'https://example.com', status: 'active', credential_configured: true, created_at: '2026-08-31T00:00:00Z', updated_at: '2026-08-31T00:00:00Z' }])
    vi.mocked(teacherAPI.catalogCandidates).mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    vi.mocked(teacherAPI.probeModel).mockReset()
  })

  it('opens a quick-fill decision after entering step two and keeps complete defaults when declined', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('全部模型 1')
    await wrapper.get('button.button.primary').trigger('click')
    const textInputs = wrapper.findAll('input').filter(input => input.attributes('type') !== 'checkbox')
    await textInputs[1].setValue('new-model')
    await textInputs[2].setValue('New Model')
    expect(wrapper.findAll('button').some(button => button.text() === '下一步')).toBe(false)
    const baseStep = wrapper.findAll('button').find(button => button.text().includes('基础配置'))!
    expect(baseStep.attributes('disabled')).toBeUndefined()
    await baseStep.trigger('click')
    expect(wrapper.findAll('[role="dialog"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('好消息：')
    expect(wrapper.text()).toContain('选否则会采用默认值进入下一步')
    await wrapper.findAll('button').find(button => button.text() === '否，采用默认值')!.trigger('click')
    expect(wrapper.findAll('[role="dialog"]')).toHaveLength(1)
    expect(wrapper.find('textarea').element.value).toBe('New Model（new-model）')
    expect(wrapper.findAll('input').map(input => input.element.value)).toEqual(expect.arrayContaining(['128', '4', '1.000']))
  })

  it('prefills model id, applies probe configuration, closes the probe dialog and never renders response bodies', async () => {
    vi.mocked(teacherAPI.probeModel).mockResolvedValue({ results: [{ endpoint: 'models', path: '/v1/models', http_status: 200, duration_ms: 12, truncated: false, limits: { context_window: 256000 } }], configuration: { description: 'Provider description', category: 'multimodal', capabilities: ['reasoning', 'vision'], input_modalities: ['text', 'image'], output_modalities: ['text'], context_window: 256000, max_input_tokens: 240000, max_output_tokens: 16000 } })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('button.button.primary').trigger('click')
    const textInputs = wrapper.findAll('input').filter(input => input.attributes('type') !== 'checkbox')
    await textInputs[1].setValue('glm-5.1')
    await textInputs[2].setValue('GLM 5.1')
    await wrapper.findAll('button').find(button => button.text().includes('基础配置'))!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === '是，开始一键填充')!.trigger('click')
    const probeDialog = wrapper.findAll('[role="dialog"]').find(dialog => dialog.attributes('aria-label') === '一键配置')!
    expect((probeDialog.find('input[readonly]').element as HTMLInputElement).value).toBe('glm-5.1')
    expect(probeDialog.findAll('input').some(input => input.element.value === 'glm-5.1')).toBe(true)
    await probeDialog.find('select').setValue('provider-1')
    await probeDialog.find('form').trigger('submit')
    await flushPromises()
    expect(teacherAPI.probeModel).toHaveBeenCalledWith(expect.objectContaining({ provider_id: 'provider-1', upstream_model_name: 'glm-5.1' }))
    expect(wrapper.findAll('[role="dialog"]')).toHaveLength(1)
    expect(wrapper.find('textarea').element.value).toBe('Provider description')
    expect(wrapper.text()).not.toContain('HTTP 200')
    expect(wrapper.findAll('input').map(input => input.element.value)).toEqual(expect.arrayContaining(['256', '240', '16']))
  })

  it('opens binding creation as a dialog from the plus button', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('Binding'))!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === '＋')!.trigger('click')
    const dialogs = wrapper.findAll('[role="dialog"]')
    expect(dialogs).toHaveLength(2)
    expect(dialogs.some(dialog => dialog.attributes('aria-label') === '新增 Binding')).toBe(true)
  })
})
