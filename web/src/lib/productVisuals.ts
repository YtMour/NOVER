const demoVisuals: Record<string, { src: string; alt: string }> = {
  'demo-nover-starter': {
    src: '/images/nover-starter-editorial-v4.webp',
    alt: '层叠纸张与模块构成的 NOVER Starter Kit 编辑插画',
  },
  'demo-react-interface': {
    src: '/images/nover-react-editorial-v4.webp',
    alt: '由组件、色板与布局模块组成的 React Interface Pack 编辑插画',
  },
  'demo-go-api-blueprint': {
    src: '/images/nover-go-editorial-v4.webp',
    alt: '以节点与连接关系表现 Go API Blueprint 的技术插画',
  },
}

const genericVisuals = [
  { src: '/images/nover-starter-editorial-v4.webp', alt: '层叠纸张与模块构成的数字产品编辑插画' },
  { src: '/images/nover-react-editorial-v4.webp', alt: '由组件、色板与布局模块组成的数字产品编辑插画' },
  { src: '/images/nover-go-editorial-v4.webp', alt: '以节点与连接关系表现数字产品结构的技术插画' },
]

export function getProductVisual(productId: string, index = 0) {
  return demoVisuals[productId] ?? genericVisuals[index % genericVisuals.length]
}

export function getDemoProductVisual(productId: string) {
  return demoVisuals[productId]
}
