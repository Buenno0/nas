import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { streamUrl } from './api'

export interface Track {
  id: number
  name: string
  artist?: string
  album?: string
  poster?: string
  duration: number
}

interface PlayerContextValue {
  queue: Track[]
  index: number
  current?: Track
  playing: boolean
  position: number
  duration: number
  volume: number
  play: (queue: Track[], index?: number) => void
  toggle: () => void
  next: () => void
  previous: () => void
  seek: (seconds: number) => void
  setVolume: (value: number) => void
  close: () => void
}

const PlayerContext = createContext<PlayerContextValue | null>(null)

/** O <audio> vive aqui, fora das telas: trocar de página não corta a música. */
export function PlayerProvider({ children }: { children: ReactNode }) {
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const [queue, setQueue] = useState<Track[]>([])
  const [index, setIndex] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [position, setPosition] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolumeState] = useState(1)

  const current = queue[index]

  useEffect(() => {
    if (!audioRef.current) audioRef.current = new Audio()
    const audio = audioRef.current

    const onTime = () => setPosition(audio.currentTime)
    const onMeta = () => setDuration(audio.duration || 0)
    const onPlay = () => setPlaying(true)
    const onPause = () => setPlaying(false)
    const onEnded = () => setIndex((i) => (i + 1 < queue.length ? i + 1 : i))

    audio.addEventListener('timeupdate', onTime)
    audio.addEventListener('loadedmetadata', onMeta)
    audio.addEventListener('play', onPlay)
    audio.addEventListener('pause', onPause)
    audio.addEventListener('ended', onEnded)
    return () => {
      audio.removeEventListener('timeupdate', onTime)
      audio.removeEventListener('loadedmetadata', onMeta)
      audio.removeEventListener('play', onPlay)
      audio.removeEventListener('pause', onPause)
      audio.removeEventListener('ended', onEnded)
    }
  }, [queue.length])

  // Troca de faixa: aponta o src novo e toca.
  useEffect(() => {
    const audio = audioRef.current
    if (!audio || !current) return
    const src = streamUrl(current.id)
    if (!audio.src.endsWith(src)) {
      audio.src = src
      audio.play().catch(() => setPlaying(false))
    }
  }, [current])

  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume
  }, [volume])

  const play = useCallback((tracks: Track[], startAt = 0) => {
    setQueue(tracks)
    setIndex(startAt)
    setPosition(0)
    const audio = audioRef.current
    const track = tracks[startAt]
    if (audio && track) {
      audio.src = streamUrl(track.id)
      audio.play().catch(() => setPlaying(false))
    }
  }, [])

  const toggle = useCallback(() => {
    const audio = audioRef.current
    if (!audio || !current) return
    if (audio.paused) void audio.play()
    else audio.pause()
  }, [current])

  const next = useCallback(() => setIndex((i) => (i + 1 < queue.length ? i + 1 : i)), [queue.length])

  const previous = useCallback(() => {
    const audio = audioRef.current
    // Padrão de player: primeiro clique volta ao começo da faixa.
    if (audio && audio.currentTime > 3) {
      audio.currentTime = 0
      return
    }
    setIndex((i) => (i > 0 ? i - 1 : 0))
  }, [])

  const seek = useCallback((seconds: number) => {
    if (audioRef.current) audioRef.current.currentTime = seconds
  }, [])

  const close = useCallback(() => {
    const audio = audioRef.current
    if (audio) {
      audio.pause()
      audio.removeAttribute('src')
      audio.load()
    }
    setQueue([])
    setIndex(0)
    setPlaying(false)
  }, [])

  const value = useMemo<PlayerContextValue>(
    () => ({
      queue,
      index,
      current,
      playing,
      position,
      duration,
      volume,
      play,
      toggle,
      next,
      previous,
      seek,
      setVolume: setVolumeState,
      close,
    }),
    [queue, index, current, playing, position, duration, volume, play, toggle, next, previous, seek, close],
  )

  return <PlayerContext.Provider value={value}>{children}</PlayerContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function usePlayer() {
  const ctx = useContext(PlayerContext)
  if (!ctx) throw new Error('usePlayer precisa estar dentro de PlayerProvider')
  return ctx
}
