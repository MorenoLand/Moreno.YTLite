import { System, Window } from '@wailsio/runtime'
import { YtLiteService } from '../bindings/github.com/MorenoLand/Moreno.YTLite/index.js'
import { BlockedItem } from '../bindings/github.com/MorenoLand/Moreno.YTLite/models.js'

type Arguments = Record<string, unknown>

function objectArguments(value: unknown): Arguments {
  return value && typeof value === 'object' ? value as Arguments : {}
}

function stringArgument(value: unknown): string { return typeof value === 'string' ? value : '' }

export function invoke<T = unknown>(command: string, args?: unknown): Promise<T> {
  const values = objectArguments(args)
  let result: unknown
  switch (command) {
    case 'drag_window': System.invoke('wails:drag'); result = undefined; break
    case 'minimize_window': result = Window.Minimise(); break
    case 'hide_window': result = Window.Hide(); break
    case 'toggle_maximize': result = Window.ToggleMaximise(); break
    case 'set_input_lock': result = YtLiteService.SetInputLock(Boolean(values.locked)); break
    case 'search_youtube': result = YtLiteService.SearchYouTube(stringArgument(values.query)); break
    case 'search_youtube_more': result = YtLiteService.SearchYouTubeMore(stringArgument(values.cursor)); break
    case 'load_home': result = YtLiteService.LoadHome(); break
    case 'load_shorts': result = YtLiteService.LoadShorts(); break
    case 'load_shorts_more': result = YtLiteService.LoadShortsMore(stringArgument(values.videoId)); break
    case 'load_channel_videos': result = YtLiteService.LoadChannelVideos(stringArgument(values.channel), stringArgument(values.channelId)); break
    case 'load_subscription_avatar': result = YtLiteService.LoadSubscriptionAvatar(stringArgument(values.channel), stringArgument(values.channelId)); break
    case 'list_subscriptions': result = YtLiteService.ListSubscriptions(); break
    case 'subscribe_channel': result = YtLiteService.SubscribeChannel(stringArgument(values.channel), stringArgument(values.channelId), stringArgument(values.avatar)); break
    case 'unsubscribe_channel': result = YtLiteService.UnsubscribeChannel(stringArgument(values.channel)); break
    case 'list_blocks': result = YtLiteService.ListBlocks(); break
    case 'block_video': result = YtLiteService.BlockVideo(stringArgument(values.id), stringArgument(values.label), stringArgument(values.thumbnail)); break
    case 'block_channel': result = YtLiteService.BlockChannel(stringArgument(values.channel), stringArgument(values.channelId), stringArgument(values.thumbnail)); break
    case 'unblock_item': result = YtLiteService.UnblockItem(new BlockedItem({ kind: stringArgument(values.kind), value: stringArgument(values.value), label: stringArgument(values.label), thumbnail: stringArgument(values.thumbnail) })); break
    default: return Promise.reject(new Error(`Unknown command: ${command}`))
  }
  return Promise.resolve(result as T)
}
