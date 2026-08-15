import { Route, Routes } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError } from './lib/api'
import { Layout } from './components/Layout'
import { Spinner } from './components/states'
import { Login } from './pages/Login'
import { Home } from './pages/Home'
import { Library, Search } from './pages/Library'
import { Title } from './pages/Title'
import { Settings } from './pages/Settings'
import { Watch } from './pages/Watch'
import { NaoEncontrado, Ruinas } from './pages/Ruinas'

export default function App() {
  const { data: user, isLoading, isError, error } = useQuery({
    queryKey: ['me'],
    queryFn: api.me,
    retry: false,
  })

  if (isLoading) {
    return (
      <div className="grid min-h-full place-items-center">
        <Spinner label="Conectando…" />
      </div>
    )
  }

  // Qualquer 401 cai no login; outros erros também, já que sem sessão não há
  // nada a mostrar. As ruínas são a exceção: existem para serem visitadas por
  // quem ainda nem entrou.
  if (isError || !user) {
    if (error instanceof ApiError && error.status !== 401) {
      console.error('falha ao consultar a sessão:', error)
    }
    return (
      <Routes>
        <Route path="/ruinas" element={<Ruinas />} />
        <Route path="*" element={<Login />} />
      </Routes>
    )
  }

  return (
    <Routes>
      {/* O player ocupa a tela inteira, fora do layout com barras. */}
      <Route path="/watch/:fileId" element={<Watch />} />
      <Route
        path="*"
        element={
          <Layout>
            <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/library/:id" element={<Library />} />
              <Route path="/search" element={<Search />} />
              <Route path="/title/:id" element={<Title />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="/ruinas" element={<Ruinas />} />
              {/* Rota desconhecida dentro do app: o servidor já devolve a tela
                  de 404 no carregamento direto, isto cobre a navegação interna. */}
              <Route path="*" element={<NaoEncontrado />} />
            </Routes>
          </Layout>
        }
      />
    </Routes>
  )
}
