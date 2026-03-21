declare type BlobCursorProps = {
  blobType?: 'circle' | string
  fillColor?: string
  trailCount?: number
  sizes?: number[]
  innerSizes?: number[]
  innerColor?: string
  opacities?: number[]
  shadowColor?: string
  shadowBlur?: number
  shadowOffsetX?: number
  shadowOffsetY?: number
  filterId?: string
  filterStdDeviation?: number
  filterColorMatrixValues?: string
  useFilter?: boolean
  fastDuration?: number
  slowDuration?: number
  fastEase?: string
  slowEase?: string
  zIndex?: number
}

declare function BlobCursor(props: BlobCursorProps): import('react/jsx-runtime').JSX.Element

export default BlobCursor
