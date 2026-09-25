import { Route, Routes } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError } from "./lib/api";
import { Layout } from "./components/Layout";
import { Spinner } from "./components/states";
import { Login } from "./pages/Login";
import { Home } from "./pages/Home";
import { Library, Search } from "./pages/Library";
import { Title } from "./pages/Title";
import { Settings } from "./pages/Settings";
import { Metrics } from "./pages/Metrics";
import { Tecnico } from "./pages/Tecnico";
import { Artista, Artistas } from "./pages/Artistas";
import { Colecao, Colecoes } from "./pages/Colecoes";
import { Watch } from "./pages/Watch";
import { NaoEncontrado, Ruinas } from "./pages/Ruinas";
import { ConectarTV } from "./pages/ConectarTV";
import { EnviosProvider } from "./lib/envios";

export default function App() {
  const {
    data: user,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["me"],
    queryFn: api.me,
    retry: false,
  });

  if (isLoading) {
    return (
      <div className="grid min-h-full place-items-center">
        <Spinner label="Conectando…" />
      </div>
    );
  }

  // Qualquer 401 cai no login; outros erros também, já que sem sessão não há
  // nada a mostrar. As ruínas são a exceção: existem para serem visitadas por
  // quem ainda nem entrou.
  if (isError || !user) {
    if (error instanceof ApiError && error.status !== 401) {
      console.error("falha ao consultar a sessão:", error);
    }
    return (
      <Routes>
        <Route path="/ruinas" element={<Ruinas />} />
        <Route path="*" element={<Login />} />
      </Routes>
    );
  }

  // Os envios vivem acima das rotas: começar um em Configurações e ir ver o
  // acervo não o interrompe nem some com o progresso.
  return (
    <EnviosProvider>
      <Routes>
        {/* O player ocupa a tela inteira, fora do layout com barras. */}
        <Route path="/watch/:fileId" element={<Watch />} />
        <Route path="/conectar" element={<ConectarTV />} />
        <Route
          path="*"
          element={
            <Layout>
              <Routes>
                <Route path="/" element={<Home />} />
                <Route path="/library/:id" element={<Library />} />
                <Route path="/search" element={<Search />} />
                <Route path="/title/:id" element={<Title />} />
                <Route path="/colecoes" element={<Colecoes />} />
                <Route path="/colecao/:id" element={<Colecao />} />
                <Route path="/artistas" element={<Artistas />} />
                <Route path="/artista/:nome" element={<Artista />} />
                <Route path="/settings" element={<Settings />} />
                <Route path="/metricas" element={<Metrics />} />
                <Route path="/tecnico" element={<Tecnico />} />
                <Route path="/ruinas" element={<Ruinas />} />
                {/* Rota desconhecida dentro do app: o servidor já devolve a tela
                  de 404 no carregamento direto, isto cobre a navegação interna. */}
                <Route path="*" element={<NaoEncontrado />} />
              </Routes>
            </Layout>
          }
        />
      </Routes>
    </EnviosProvider>
  );
}
