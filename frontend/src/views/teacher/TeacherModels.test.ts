import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import TeacherModels from './TeacherModels.vue'

vi.mock('vue-router',()=>({useRoute:()=>({query:{}}),useRouter:()=>({replace:vi.fn()})}))
vi.mock('../../api/teacher',()=>({teacherAPI:{models:vi.fn().mockResolvedValue([{model_id:'m1',display_name:'M1',description:null,category:'text',capabilities:[],input_modalities:[],output_modalities:[],context_window:null,max_input_tokens:null,max_output_tokens:null,is_common:true,status:'active',route_ready:true,credit_multiplier:'1.000'}]),providers:vi.fn().mockResolvedValue([]),model:vi.fn(),catalogCandidates:vi.fn(),createModel:vi.fn(),updateModel:vi.fn(),createBinding:vi.fn(),updateBinding:vi.fn(),probeModel:vi.fn()}}))

describe('TeacherModels',()=>{it('shows total model count and four wizard steps',async()=>{const wrapper=mount(TeacherModels,{global:{stubs:{LoadingRegion:{template:'<div><slot/></div>'},AppDialog:{props:['open','title'],template:'<div v-if="open"><slot/></div>'},StatusBadge:true}}});await flushPromises();expect(wrapper.text()).toContain('全部模型 1');await wrapper.get('button.button.primary').trigger('click');for(const label of ['模型标识','基础配置','Binding','启用确认'])expect(wrapper.text()).toContain(label)})})
