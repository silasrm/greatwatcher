package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon"
	"github.com/joho/godotenv"
	"github.com/radovskyb/watcher"
	"github.com/rs/zerolog"
	"golang.org/x/net/html/charset"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

var (
	buildInfo, _ = debug.ReadBuildInfo()

	logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		FormatLevel: func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("[%s]", i))
		},
		FormatMessage: func(i interface{}) string {
			return fmt.Sprintf("| %s |", i)
		},
		FormatCaller: func(i interface{}) string {
			return filepath.Base(fmt.Sprintf("%s", i))
		},
		PartsExclude: []string{
			zerolog.TimestampFieldName,
		},
	}).
		Level(zerolog.TraceLevel).
		With().
		Timestamp().
		Caller().
		Int("pid", os.Getpid()).
		Str("go_version", buildInfo.GoVersion).
		Logger()
)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func folderExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func extractFile(data Resultado, outputDir string) string {
	//sDec, _ := b64.StdEncoding.DecodeString(data.Pdf)
	//logger.Println(string(sDec))
	//logger.Println()
	dec, err := base64.StdEncoding.DecodeString(data.Pdf)
	if err != nil {
		panic(err)
	}

	filename := outputDir + data.Prescricao + ".pdf"
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err := f.Write(dec); err != nil {
		panic(err)
	}
	if err := f.Sync(); err != nil {
		panic(err)
	}

	return filename
}

func getXml(path string) (Resultado, error) {
	// Open our xmlFile
	xmlFile, err := os.Open(path)
	// if we os.Open returns an error then handle it
	if err != nil {
		logger.Println(err)
	}

	logger.Info().Msgf("\t-> Abrindo %s...", path)
	// defer the closing of our xmlFile so that we can parse it later on
	defer xmlFile.Close()

	// read our opened xmlFile as a byte array.
	byteValue, err := io.ReadAll(xmlFile)
	if err != nil {
		logger.Println(err)
	}

	resultado := Resultado{}

	reader := bytes.NewReader(byteValue)
	decoder := xml.NewDecoder(reader)
	decoder.CharsetReader = charset.NewReaderLabel
	err = decoder.Decode(&resultado)
	if err != nil {
		logger.Println(err)
	}

	return resultado, err
}

func UploadFile(url string, params map[string]string, files ...File) (Response, error) {
	var (
		buf = new(bytes.Buffer)
		w   = multipart.NewWriter(buf)
	)

	for _, f := range files {
		part, err := w.CreateFormFile(f.Name, filepath.Base(f.Filename))
		if err != nil {
			return Response{}, err
		}

		_, err = part.Write(f.File)
		if err != nil {
			return Response{}, err
		}
	}

	for key, val := range params {
		_ = w.WriteField(key, val)
	}

	err := w.Close()
	if err != nil {
		return Response{}, err
	}

	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return Response{}, err
	}
	req.Header.Add("Content-Type", w.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	//if err != nil {
	//	return []byte{}, err
	//}
	//defer res.Body.Close()
	//
	//cnt, err := io.ReadAll(res.Body)
	//if err != nil {
	//	return []byte{}, err
	//}
	//
	//return cnt, nil

	if err != nil {
		return Response{}, err
	}

	//body := &bytes.Buffer{}
	//_, err = body.ReadFrom(resp.Body)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//resp.Body.Close()
	////fmt.Println(resp.StatusCode)
	////fmt.Println(resp.Header)
	////fmt.Println(body)
	//
	//bodyBuffer := []byte(body)
	//bodyBuffer := new(bytes.Buffer)
	//bodyBuffer.Write(body)
	//json.Unmarshal(bodyBuffer, &result)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	//logger.Println(string(body))
	result := Response{}
	json.Unmarshal(body, &result)

	return result, nil
}

func MoveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %v", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("Couldn't open dest file: %v", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return fmt.Errorf("Couldn't copy to dest from source: %v", err)
	}

	inputFile.Close() // for Windows, close before trying to remove: https://stackoverflow.com/a/64943554/246801

	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't remove source file: %v", err)
	}
	return nil
}

type Prescricao struct {
	XMLName          xml.Name `xml:"prescricao"`
	NumeroPrescricao string   `xml:"NR_PRESCRICAO"`
	DataPrescricao   string   `xml:"DT_PRESCRICAO"`
	NomePaciente     string   `xml:"NM_PACIENTE"`
	DataNascimento   string   `xml:"DT_NASCIMENTO"`
	Sexo             string   `xml:"IE_SEXO"`
	Logradouro       string   `xml:"DS_ENDERECO"`
	Numero           string   `xml:"NR_ENDERECO"`
	Complemento      string   `xml:"DS_COMPLEMENTO"`
	Bairro           string   `xml:"DS_BAIRRO"`
	Municipio        string   `xml:"DS_MUNICIPIO"`
	Estado           string   `xml:"SG_ESTADO"`
	Cep              string   `xml:"CD_CEP"`
	Telefone         string   `xml:"NR_TELEFONE"`
	Cpf              string   `xml:"NR_CPF"`
	Unidade          string   `xml:"CD_UNIDADE"`
	Crm              string   `xml:"NR_CRM"`
	Medico           string   `xml:"NM_MEDICO"`
	CodigoConvenio   string   `xml:"CD_CONVENIO"`
	Convenio         string   `xml:"DS_CONVENIO"`
	Plano            string   `xml:"DS_PLANO_CONVENIO"`
	NumeroProntuario string   `xml:"NR_PRONTUARIO"`
	Urgencia         string   `xml:"IE_URGENCIA"`
	CodigoOrigem     string   `xml:"CD_ORIGEM"`
	Origem           string   `xml:"DS_ORIGEM"`
	Usuario          string   `xml:"NM_USUARIO"`
	Exames           Exames   `xml:"EXAMES"`
}

type Exames struct {
	XMLName xml.Name `xml:"EXAMES"`
	Exame   string   `xml:"EXAME"`
}

type Resultado struct {
	XMLName       xml.Name `xml:"prescricao"`
	Prescricao    string   `xml:"NR_PRESCRICAO"`
	Exame         string   `xml:"EXAME"`
	DataLiberacao string   `xml:"DATA_LIBERACAO"`
	Pdf           string   `xml:"PDF"`
	PdfPath       string
}

type File struct {
	Name     string
	Filename string
	File     []byte
}

type Response struct {
	Success string `json:"success"`
	Error   string `json:"error"`
}

func main() {
	onExit := func() {
		//now := time.Now()
		//os.WriteFile(fmt.Sprintf(`on_exit_%d.txt`, now.UnixNano()), []byte(now.String()), 0644)
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	err := godotenv.Load()
	if err != nil {
		logger.Error().Msg("Error loading .env file")
	}

	err = beeep.Beep(beeep.DefaultFreq, beeep.DefaultDuration)
	if err != nil {
		panic(err)
	}

	appName := os.Getenv("APPNAME")
	resultPath := os.Getenv("RESULT_PATH")
	resultDispatchedPath := os.Getenv("RESULT_DISPATCHED_PATH")
	resultErrorPath := os.Getenv("RESULT_ERROR_PATH")
	serverUploadUrl := os.Getenv("SERVER_UPLOAD_URL")
	userId := os.Getenv("USER_ID")
	appToken := os.Getenv("APP_TOKEN")
	formFileFieldname := os.Getenv("FORM_FILE_FIELDNAME")

	////////
	systray.SetTemplateIcon(icon.Data, icon.Data)
	//systray.SetTitle(appName)
	systray.SetTooltip(appName)
	mQuitOrig := systray.AddMenuItem("Sair", "Fechar o aplicativo")
	go func() {
		<-mQuitOrig.ClickedCh
		fmt.Println("Requesting quit")
		systray.Quit()
		fmt.Println("Finished quitting")
	}()
	////////

	if !folderExists(resultPath) {
		logger.Error().Msgf("Folder does not exist: %s", resultPath)
	} else {
		logger.Info().Msgf("Folder found: %s", resultPath)
	}

	w := watcher.New()

	// SetMaxEvents to 1 to allow at most 1 event's to be received
	// on the Event channel per watching cycle.
	//
	// If SetMaxEvents is not set, the default is to send all events.
	w.SetMaxEvents(1)

	// Only notify rename and move events.
	//w.FilterOps(watcher.Rename, watcher.Move)
	w.FilterOps(watcher.Rename, watcher.Move, watcher.Create)

	// Only files that match the regular expression during file listings
	// will be watched.
	r := regexp.MustCompile("^*.xml$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))

	go func() {
		for {
			select {
			case event := <-w.Event:
				if !event.IsDir() && event.Path != "-" {
					size := uint64(event.Size())
					logger.Info().Msgf("Evento: %s", event.Op)
					logger.Info().Msgf("\tArquivo: %s", event.Name())
					logger.Info().Msgf("\tTamanho: %s", humanize.Bytes(size))
					logger.Info().Msgf("\tCaminho: %s", event.Path)

					if event.Op == watcher.Create {
						err := beeep.Notify("GreatWatcher", "Novo resultado adicionado à pasta: "+event.Name(), "assets/information.png")
						if err != nil {
							panic(err)
						}
					}

					r, err := getXml(event.Path)
					if err != nil {
						err = beeep.Alert("GreatWatcher - "+event.Name(), "Erro ao ler o XML", "assets/warning.png")
						if err != nil {
							panic(err)
						}

						logger.Error().Msg("\t\t-> Erro ao ler o XML")
					}

					logger.Info().Msgf("\t\t-> ID: %s", r.Prescricao)

					if len(r.Pdf) > 0 {
						logger.Info().Msg("\t\tCOM PDF!")

						pdfPath := extractFile(r, resultPath)

						if len(pdfPath) > 0 {
							r.PdfPath = pdfPath
						}

						logger.Info().Msgf("\t\t--> Extraido em: %s", r.PdfPath)

						err = beeep.Notify("GreatWatcher - "+event.Name(), "PDF extraído", "assets/information.png")
						if err != nil {
							panic(err)
						}

						extraParams := map[string]string{
							"id":         r.Prescricao,
							"exame":      r.Exame,
							"data":       r.DataLiberacao,
							"usuario_id": userId,
							"token":      appToken,
						}

						fileContent, err := os.Open(r.PdfPath)
						// if we os.Open returns an error then handle it
						if err != nil {
							logger.Println(err)
						}

						fileContentBytes, err := io.ReadAll(fileContent)
						if err != nil {
							logger.Println(err)
						}

						file := File{
							//Name:      event.Name(),
							Name: formFileFieldname,
							//Extension: "pdf",
							Filename: r.PdfPath,
							File:     fileContentBytes,
						}

						result, err := UploadFile(serverUploadUrl+"id/"+r.Prescricao, extraParams, file)
						if err != nil {
							logger.Println(err)
						}

						logger.Println(result)
						if len(result.Success) > 0 {
							err = beeep.Notify("GreatWatcher - "+event.Name(), "Resultado enviado para o servidor! "+result.Success, "assets/information.png")
							if err != nil {
								panic(err)
							}

							err = MoveFile(event.Path, resultDispatchedPath+filepath.Base(event.Path))
							if err != nil {
								log.Fatal(err)
							}

							logger.Info().Msgf("\t\t--> Movido de %s para %s", event.Path, resultDispatchedPath+filepath.Base(event.Path))

							err = MoveFile(r.PdfPath, resultDispatchedPath+filepath.Base(r.PdfPath))
							if err != nil {
								log.Fatal(err)
							}

							logger.Info().Msgf("\t\t--> Movido de %s para %s", r.PdfPath, resultDispatchedPath+filepath.Base(r.PdfPath))
						} else {
							err = MoveFile(event.Path, resultErrorPath+filepath.Base(event.Path))
							if err != nil {
								log.Fatal(err)
							}

							logger.Info().Msgf("\t\t--> Movido de %s para %s", event.Path, resultErrorPath+filepath.Base(event.Path))

							err = MoveFile(r.PdfPath, resultErrorPath+filepath.Base(r.PdfPath))
							if err != nil {
								log.Fatal(err)
							}

							logger.Info().Msgf("\t\t--> Movido de %s para %s", r.PdfPath, resultErrorPath+filepath.Base(r.PdfPath))

							err = beeep.Alert("GreatWatcher - "+event.Name(), result.Error, "assets/warning.png")
							if err != nil {
								panic(err)
							}
						}
					} else {
						err = beeep.Alert("GreatWatcher - "+event.Name(), "Sem PDF do resultado", "assets/warning.png")
						if err != nil {
							panic(err)
						}

						logger.Info().Msg("\t\tSem PDF!")
					}
				}
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch this folder for changes.
	if err := w.Add(resultPath); err != nil {
		log.Fatalln(err)
	}

	// Print a list of all of the files and folders currently
	// being watched and their paths.
	for path, f := range w.WatchedFiles() {
		logger.Info().Msgf("%s: %s\n", path, f.Name())
	}

	logger.Println()

	// Trigger 2 events after watcher started.
	go func() {
		w.Wait()
		w.TriggerEvent(watcher.Create, nil)
		w.TriggerEvent(watcher.Remove, nil)
	}()

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}
}
