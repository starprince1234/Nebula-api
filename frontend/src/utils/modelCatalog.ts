import type { ModelCapability } from '../api/types'
export const capabilityOptions:Array<{value:ModelCapability;label:string}>=[['reasoning','推理'],['vision','视觉理解'],['tool_calling','工具调用'],['structured_output','结构化输出'],['web_search','联网搜索'],['coding','代码能力'],['embeddings','Embedding'],['rerank','Rerank'],['realtime','实时交互'],['image_generation','图像生成'],['video_generation','视频生成'],['speech_to_text','语音识别'],['text_to_speech','语音合成']].map(([value,label])=>({value:value as ModelCapability,label}))
export function capabilityLabel(value:string){return capabilityOptions.find(option=>option.value===value)?.label??value}
export function formatTokenK(value:number|null|undefined){if(!value)return'—';return`${Number((value/1000).toFixed(3))}K`}
export function parseTokenK(value:string){const parsed=Number(value);return Number.isFinite(parsed)&&parsed>0?Math.round(parsed*1000):null}
