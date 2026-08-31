import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import StudentKeys from './StudentKeys.vue'

vi.mock('../../api/student',()=>({studentAPI:{organizations:vi.fn().mockResolvedValue([]),models:vi.fn().mockResolvedValue([{model_id:'common-model',display_name:'Common Model',description:null,category:'text',capabilities:[],input_modalities:['text'],output_modalities:['text'],context_window:null,max_input_tokens:null,max_output_tokens:null,is_common:true,status:'active',route_ready:true,credit_multiplier:'1.000'}]),keys:vi.fn().mockResolvedValue([]),projects:vi.fn(),submit:vi.fn(),key:vi.fn(),claim:vi.fn()}}))

describe('StudentKeys',()=>{it('shows common models and the simplified new-model form',async()=>{const wrapper=mount(StudentKeys,{global:{stubs:{LoadingRegion:{template:'<div><slot/></div>'},AppDialog:{props:['open','title'],template:'<div v-if="open"><slot/></div>'},StatusProgress:true,StatusBadge:true}}});await flushPromises();expect(wrapper.text()).toContain('Common Model');await wrapper.findAll('button').find(button=>button.text()==='申请新模型')!.trigger('click');expect(wrapper.text()).toContain('模型名称（选填）');expect(wrapper.text()).not.toContain('输入模态')})})
