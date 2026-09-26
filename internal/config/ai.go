package config

import "strings"

// BuildAIArgv construye el argv del comando AI a partir de la plantilla de la
// config global y el prompt del marcador.
//
// El límite de seguridad vive aquí: el prompt entra como UN elemento de argv
// (nunca interpolado en un `sh -c`), así que puede contener espacios, saltos de
// línea, comillas o `$` sin que nada los interprete.
//
// vars son los placeholders de contexto ({branch}, {behind}, …) ya resueltos
// por quien llama: config no importa gitstatus para no crear un ciclo
// config → gitstatus → discovery → config.
//
// La separación por campos comparte la limitación ya aceptada de CmdArgs: no se
// pueden expresar argumentos con espacios dentro de comillas. El reemplazo es
// de una sola pasada (strings.Replacer no re-escanea lo insertado), así que un
// prompt que contenga `{branch}` queda literal en vez de expandirse en cascada.
func BuildAIArgv(template, prompt string, vars map[string]string) []string {
	fields := strings.Fields(template)
	if len(fields) == 0 {
		return nil
	}
	pairs := make([]string, 0, 2*(len(vars)+1))
	pairs = append(pairs, "{prompt}", prompt)
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	replacer := strings.NewReplacer(pairs...)

	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = replacer.Replace(f)
	}
	return out
}
