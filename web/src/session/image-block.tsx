import type { Block } from "@/session/state"

/** An image from a message or a tool result, scaled down to the transcript's width. */
export function ImageBlock(props: {
  image: Extract<Block, { type: "image" }>
  alt: string
}) {
  return (
    <img
      src={`data:${props.image.media_type};base64,${props.image.data}`}
      alt={props.alt}
      className="h-auto max-w-full rounded-md border"
    />
  )
}
