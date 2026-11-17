package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Arseniy22877/game/gameengine"
)

const (
	scenariosDirectory = "scenarios"
	maxScenarioSize    = 5 << 20
	maxImageSize       = 10 << 20
)

type scenarioResponse struct {
	Name string          `json:"name"`
	Game json.RawMessage `json:"game"`
}

func main() {
	if err := os.MkdirAll(scenariosDirectory, 0755); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/scenarios", handleScenarios)
	mux.HandleFunc("/api/scenario", handleScenario)
	mux.HandleFunc("/api/image", handleImage)
	if err := registerGameplayRoutes(mux); err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:              ":8090",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("scenario editor is available at http://localhost:8090")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(indexHTML))
}

func handleScenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := os.MkdirAll(scenariosDirectory, 0755); err != nil {
		http.Error(w, "cannot create scenarios directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	entries, err := os.ReadDir(scenariosDirectory)
	if err != nil {
		http.Error(w, "cannot read scenarios: "+err.Error(), http.StatusInternalServerError)
		return
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			names = append(names, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		}
	}
	sort.Strings(names)
	writeJSON(w, names)
}

func handleScenario(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	path, err := scenarioPath(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		game, err := gameengine.CreateGameJSON(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "scenario not found", http.StatusNotFound)
				return
			}
			http.Error(w, "invalid scenario: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		pretty, err := gameengine.PrettyGameJSON(game)
		if err != nil {
			http.Error(w, "cannot encode scenario: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, scenarioResponse{Name: name, Game: pretty})

	case http.MethodPost:
		if err := os.MkdirAll(scenariosDirectory, 0755); err != nil {
			http.Error(w, "cannot create scenarios directory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			if os.IsExist(err) {
				http.Error(w, "scenario already exists", http.StatusConflict)
				return
			}
			http.Error(w, "cannot create scenario: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := file.Close(); err != nil {
			http.Error(w, "cannot finish creating scenario: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"status": "created", "name": name})

	case http.MethodPut:
		saveScenario(w, r, name, path)

	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func saveScenario(w http.ResponseWriter, r *http.Request, name, path string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxScenarioSize)
	decoder := json.NewDecoder(r.Body)

	var request scenarioResponse
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := ensureRequestEOF(decoder); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if request.Name != "" && request.Name != name {
		http.Error(w, "scenario name cannot be changed", http.StatusBadRequest)
		return
	}

	game, err := gameengine.ParseGameJSON(request.Game)
	if err != nil {
		http.Error(w, "invalid game JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := gameengine.ValidateGame(&game); err != nil {
		http.Error(w, "game validation failed: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := gameengine.SaveGameJSON(path, game); err != nil {
		http.Error(w, "cannot save scenario: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "saved", "name": name})
}

func handleImage(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, err := scenarioPath(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	imageDirectory := filepath.Join(scenariosDirectory, name, "images")
	switch r.Method {
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, maxImageSize+(1<<20))
		if err := r.ParseMultipartForm(maxImageSize); err != nil {
			http.Error(w, "invalid image upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}

		file, _, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "image is required: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, maxImageSize+1))
		if err != nil {
			http.Error(w, "cannot read image: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(data) == 0 || len(data) > maxImageSize {
			http.Error(w, "image must be between 1 byte and 10 MB", http.StatusBadRequest)
			return
		}
		extension, contentType := imageType(data)
		if extension == "" {
			http.Error(w, "unsupported image format; use PNG, JPEG, WebP or GIF", http.StatusBadRequest)
			return
		}

		if err := os.MkdirAll(imageDirectory, 0700); err != nil {
			http.Error(w, "cannot create image directory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		filename := safeImageName(extension)
		target := filepath.Join(imageDirectory, filename)
		if err := os.WriteFile(target, data, 0600); err != nil {
			http.Error(w, "cannot save image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		writeJSON(w, map[string]string{"imageUrl": "images/" + filename, "contentType": contentType})

	case http.MethodGet:
		filename := r.URL.Query().Get("file")
		const prefix = "images/"
		if !strings.HasPrefix(filename, prefix) {
			http.Error(w, "invalid image path", http.StatusBadRequest)
			return
		}
		relative := strings.TrimPrefix(filename, prefix)
		if relative == "" || filepath.Base(relative) != relative || relative == "." || relative == ".." {
			http.Error(w, "invalid image path", http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, filepath.Join(imageDirectory, relative))

	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func imageType(data []byte) (extension, contentType string) {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return ".png", "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return ".jpg", "image/jpeg"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return ".gif", "image/gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return ".webp", "image/webp"
	default:
		return "", ""
	}
}

func safeImageName(extension string) string {
	return "event-" + strconv.FormatInt(time.Now().UnixNano(), 10) + extension
}

func scenarioPath(name string) (string, error) {
	if name == "" || strings.TrimSpace(name) != name || name == "." || name == ".." ||
		filepath.Base(name) != name || strings.ContainsAny(name, `/\\<>:"|?*`) {
		return "", fmt.Errorf("invalid scenario name")
	}
	return filepath.Join(scenariosDirectory, name+".json"), nil
}

func ensureRequestEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("cannot write JSON response: %v", err)
	}
}

const indexHTML = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Редактор сценариев</title>
<style>
:root{font:15px/1.45 system-ui,-apple-system,"Segoe UI",sans-serif;color:#202b36;background:#edf1f5;--blue:#1769aa;--border:#d8e0e8;--muted:#687888;--panel:#fff;--soft:#f5f8fb;--danger:#b33b3b}
*{box-sizing:border-box}body{margin:0 auto;max-width:1600px;padding:18px}button,input,select,textarea{font:inherit}button{border:0;border-radius:7px;padding:9px 13px;background:var(--blue);color:white;cursor:pointer}button:hover{filter:brightness(.94)}button:disabled{opacity:.5;cursor:not-allowed}.secondary{background:#667788}.danger{background:#b33b3b}.light{background:#e8eef4;color:#263746}.small-button{padding:5px 9px;font-size:13px}
header{position:sticky;top:0;z-index:5;display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:12px 0 14px;background:#edf1f5;border-bottom:1px solid #dce3ea}h1{font-size:22px;margin:0 12px 0 0}.toolbar{display:flex;gap:7px;align-items:center;flex-wrap:wrap}.toolbar select{min-width:190px}.status{margin-left:auto;color:#4f6374}.status.error{color:#a22}.status.dirty{color:#9a6500}.intro{color:var(--muted);margin:12px 0 18px}
select,input,textarea{width:100%;min-width:0;border:1px solid #cbd5df;border-radius:7px;padding:8px;background:#fff;color:#202b36}input[type=checkbox]{width:auto;accent-color:var(--blue)}input[type=file]{padding:6px}textarea{min-height:82px;resize:vertical}.field{display:flex;flex-direction:column;gap:4px;margin:8px 0}.field>span,.field-title{font-size:13px;font-weight:650;color:#47596a}.field-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:8px}.check{display:flex;align-items:center;gap:8px;margin:9px 0}.check span{font-size:14px}.panel{background:var(--panel);border:1px solid #e0e6ec;border-radius:12px;padding:16px;margin-bottom:15px;box-shadow:0 2px 10px #142b4010}.panel h2{font-size:18px;margin:0 0 12px}.panel-head{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:10px}.panel-head h2{margin:0}.muted{color:var(--muted);font-size:13px}.columns{display:grid;grid-template-columns:minmax(230px,310px) minmax(0,1fr);gap:14px;align-items:start}.event-sidebar{position:sticky;top:92px;max-height:calc(100vh - 112px);overflow:auto}.event-list{display:flex;flex-direction:column;gap:6px;margin-top:10px}.event-item{width:100%;text-align:left;background:#f4f7fa;color:#263746;border:1px solid var(--border);padding:9px 10px}.event-item.active{background:#e6f1fb;border-color:#78add8;box-shadow:inset 3px 0 var(--blue)}.event-item strong{display:block;font-size:14px}.event-item small{display:block;color:#657586;font-size:12px;margin-top:2px}.event-actions{display:flex;gap:7px;flex-wrap:wrap;margin-top:10px}.empty{padding:28px;text-align:center;color:var(--muted);border:1px dashed #cbd5df;border-radius:9px}.subpanel{background:var(--soft);border:1px solid #e3e9ef;border-radius:9px;padding:12px;margin-top:12px}.subpanel h3{font-size:15px;margin:0 0 9px}.row-list{display:flex;flex-direction:column;gap:7px}.edit-row{display:grid;grid-template-columns:minmax(140px,1fr) minmax(100px,170px) auto;gap:7px;align-items:center}.edit-row.threshold-row{grid-template-columns:minmax(150px,1fr) minmax(100px,150px) minmax(100px,130px) auto}.edit-row .check{margin:0}.remove-row{background:#f8e9e9;color:#9c2828;padding:6px 10px}.requirements{display:grid;grid-template-columns:1fr 1fr;gap:10px}.multi{min-height:100px}.attribute-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:10px}.mini-card{background:#f8fafc;border:1px solid var(--border);border-radius:9px;padding:12px}.mini-card h3{font-size:15px;margin:0 0 8px}.image-preview{display:block;max-width:260px;max-height:170px;object-fit:contain;border:1px solid var(--border);border-radius:7px;margin-top:8px;background:#f2f4f6}.weight-row{display:grid;grid-template-columns:minmax(120px,1fr) minmax(110px,180px);gap:10px;align-items:center;padding:5px 0;border-bottom:1px solid #e8edf2}.weight-row:last-child{border-bottom:0}.tag{display:inline-block;padding:2px 7px;border-radius:12px;background:#e9eef3;color:#596c7d;font-size:11px}.section-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(310px,1fr));gap:12px}.hint-secret{min-height:65px}@media(max-width:800px){body{padding:10px}.columns{grid-template-columns:1fr}.event-sidebar{position:static;max-height:none}.requirements{grid-template-columns:1fr}.edit-row,.edit-row.threshold-row{grid-template-columns:1fr 1fr auto}header{position:static}.status{margin-left:0;width:100%}}
</style>
</head>
<body>
<header>
  <h1>Редактор сценариев</h1>
  <div class="toolbar">
    <select id="scenario" aria-label="Сценарий"></select>
    <button id="new-scenario" class="secondary">Новый сценарий</button>
    <button id="reload" class="secondary">Перезагрузить</button>
    <button id="save">Сохранить</button>
  </div>
  <span id="status" class="status">Загрузка…</span>
</header>
<p class="intro">Изменения сохраняются в JSON-файл выбранного сценария. Перед сохранением движок проверяет уникальность ID, длину векторов весов и корректность ссылок между событиями, стратегиями и атрибутами. Порядок весов: базовые атрибуты, затем настраиваемые.</p>
<div id="editor"></div>
<script>
'use strict';
let game = {};
let scenarioName = '';
let selectedEventIndex = 0;
let selectedStrategyIndex = 0;
let selectedHintIndex = 0;
let dirty = false;
const $ = id => document.getElementById(id);
const select = $('scenario');
const editor = $('editor');
const status = $('status');
const arr = value => Array.isArray(value) ? value : [];
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const allAttributes = () => arr(game.BasicAttributes).concat(arr(game.ConfigurableAttributes));
const setStatus = (message, type = '') => { status.textContent = message; status.className = 'status ' + type; };
async function request(url, options) {
  const response = await fetch(url, options);
  if (!response.ok) {
    let message = await response.text();
    try { const parsed = JSON.parse(message); message = parsed.error || message; } catch (_) {}
    throw new Error(message || ('HTTP ' + response.status));
  }
  return response.json();
}
function markDirty() {
  dirty = true;
  setStatus('Есть несохранённые изменения', 'dirty');
}
function options(values, selected) {
  const list = [...arr(values)];
  if (selected !== undefined && selected !== null && selected !== '' && !list.some(value => String(value) === String(selected))) list.unshift(selected);
  return list.map(value => '<option value="' + esc(value) + '" ' + (String(value) === String(selected) ? 'selected' : '') + '>' + esc(value || '—') + '</option>').join('');
}
function idOptions(items, selected) {
  return arr(items).map(item => '<option value="' + Number(item.ID) + '" ' + (arr(selected).map(Number).includes(Number(item.ID)) ? 'selected' : '') + '>' + Number(item.ID) + ': ' + esc(item.Name || 'Без названия') + '</option>').join('');
}
function attributeOptions(selected) {
  const attributes = allAttributes();
  let html = attributes.map(attribute => '<option value="' + Number(attribute.ID) + '" ' + (Number(attribute.ID) === Number(selected) ? 'selected' : '') + '>' + Number(attribute.ID) + ' — ' + esc(attribute.Name || 'Без названия') + '</option>').join('');
  if (selected !== undefined && selected !== null && !attributes.some(attribute => Number(attribute.ID) === Number(selected))) {
    html = '<option value="' + Number(selected) + '" selected>Неизвестный атрибут ' + Number(selected) + '</option>' + html;
  }
  return html;
}
function prerequisitesHTML(value) {
  const req = value || {};
  return '<div class="subpanel"><h3>Предварительные условия</h3><div class="requirements">' +
    '<label class="field"><span>События (достаточно любого одного)</span><select class="multi req-events" multiple>' + idOptions(game.Events, req.Events) + '</select></label>' +
    '<label class="field"><span>Стратегии (достаточно любой одной активной)</span><select class="multi req-strategies" multiple>' + idOptions(game.Strategies, req.Strategies) + '</select></label>' +
    '</div><p class="muted">Внутри каждого списка действует «ИЛИ». Если оба списка заполнены, необходимо выполнить хотя бы одно условие из каждого: (любое событие) И (любая стратегия). Пустой список не накладывает ограничений. Удерживайте Ctrl (или Cmd), чтобы выбрать несколько пунктов.</p></div>';
}
function valuesFrom(element) {
  return element ? [...element.selectedOptions].map(option => Number(option.value)) : [];
}
function thresholdHTML(threshold, index) {
  return '<div class="edit-row threshold-row" data-row-index="' + index + '">' +
    '<select data-threshold-field="AttributeID" aria-label="Атрибут">' + attributeOptions(threshold.AttributeID) + '</select>' +
    '<input data-threshold-field="Min" type="number" step="any" value="' + Number(threshold.Min ?? 0) + '" aria-label="Минимальное значение" placeholder="Min">' +
    '<input data-threshold-field="Max" type="number" step="any" value="' + Number(threshold.Max ?? 0) + '" aria-label="Максимальное значение" placeholder="Max">' +
    '<button type="button" class="remove-row" data-remove-threshold="' + index + '" aria-label="Удалить порог">×</button></div>';
}
function attributeChangeHTML(change, index, prefix) {
  return '<div class="edit-row" data-row-index="' + index + '">' +
    '<select data-change-field="AttributeID" aria-label="Атрибут">' + attributeOptions(change.AttributeID) + '</select>' +
    '<input data-change-field="AttributeShift" type="number" step="any" value="' + Number(change.AttributeShift ?? 0) + '" aria-label="Изменение атрибута">' +
    '<button type="button" class="remove-row" data-remove-change="' + prefix + ':' + index + '" aria-label="Удалить изменение">×</button></div>';
}
function weightEditor(event) {
  const weights = arr(event.Weights);
  const attributes = allAttributes();
  const count = attributes.length;
  if (!count) {
    return '<p class="muted">Атрибуты не заданы. Вектор весов должен быть пустым.</p>';
  }

  let html = '';
  for (let i = 0; i < count; i++) {
    const title = attributes[i].ID + ' — ' + attributes[i].Name;
    html += '<label class="weight-row"><span>' + esc(title) + '</span><input type="number" step="any" data-weight-index="' + i + '" value="' + Number(weights[i] ?? 0) + '"></label>';
  }

  const mismatch = weights.length !== count
    ? '<p class="muted" style="color:#a22">В исходном векторе ' + weights.length + ' значений, требуется ровно ' + count + '. При сохранении будет записано ровно ' + count + ' значений; недостающие будут равны 0.</p>'
    : '';
  return html + mismatch + '<p class="muted">Порядок строго фиксирован: сначала базовые атрибуты в порядке списка, затем настраиваемые атрибуты в порядке списка. Длина Weights должна точно совпадать с общим количеством атрибутов.</p>';
}
function imageSource(url) {
  if (!url) return '';
  if (/^https?:\/\//i.test(url)) return url;
  return '/api/image?name=' + encodeURIComponent(scenarioName) + '&file=' + encodeURIComponent(url);
}
function eventEditor(event) {
  if (!event) return '<div class="empty">Добавьте первое событие, чтобы начать редактирование.</div>';
  const branches = arr(game.Branches);
  const budget = event.BudgetChange || {};
  const thresholds = arr(event.Thresholds);
  const changes = arr(event.AttributeChanges);
  const image = event.ImageURL ? '<img class="image-preview" alt="Изображение события" src="' + esc(imageSource(event.ImageURL)) + '">' : '';
  return '<div class="panel-head"><div><h2>Параметры события</h2><span class="muted">Изменения применяются к выбранному событию</span></div><span class="tag">ID ' + Number(event.ID ?? 0) + '</span></div>' +
    '<div class="field-grid">' +
      '<label class="field"><span>ID</span><input data-field="ID" type="number" step="1" value="' + Number(event.ID ?? 0) + '"></label>' +
      '<label class="field"><span>Название</span><input data-field="Name" value="' + esc(event.Name) + '"></label>' +
      '<label class="field"><span>Итерация (0 — доступно на любой итерации)</span><input data-field="Iteration" type="number" step="1" value="' + Number(event.Iteration ?? 0) + '"></label>' +
      '<label class="field"><span>Ветка</span><select data-field="Branch"><option value="">— не выбрана —</option>' + options(branches, event.Branch || '') + '</select></label>' +
      '<label class="field"><span>BasicPower</span><input data-field="BasicPower" type="number" step="any" value="' + Number(event.BasicPower ?? 0) + '"></label>' +
      '<label class="check"><input data-field="IsPositive" type="checkbox" ' + (event.IsPositive ? 'checked' : '') + '><span>Положительное событие</span></label>' +
    '</div>' +
    '<label class="field"><span>Описание</span><textarea class="description" data-field="Description">' + esc(event.Description) + '</textarea></label>' +
    '<div class="subpanel"><h3>Вес события</h3>' + weightEditor(event) + '</div>' +
    '<div class="subpanel"><h3>Изменения атрибутов</h3><div class="row-list" id="event-changes">' + changes.map((item, i) => attributeChangeHTML(item, i, 'event')).join('') + '</div><button type="button" class="light small-button" id="add-event-change">+ Добавить изменение</button></div>' +
    '<div class="subpanel"><h3>Изменение бюджета</h3><div class="field-grid">' +
      '<label class="field"><span>Деньги (MoneyShift)</span><input data-budget-field="MoneyShift" type="number" step="1" value="' + Number(budget.MoneyShift ?? 0) + '"></label>' +
      '<label class="field"><span>Очки качества (QPShift)</span><input data-budget-field="QPShift" type="number" step="1" value="' + Number(budget.QPShift ?? 0) + '"></label>' +
    '</div></div>' +
    '<div class="subpanel"><h3>Пороговые условия</h3><div class="row-list" id="event-thresholds">' + thresholds.map(thresholdHTML).join('') + '</div><button type="button" class="light small-button" id="add-threshold">+ Добавить порог</button><p class="muted">Событие подходит, когда значение атрибута находится в диапазоне от Min до Max включительно.</p></div>' +
    prerequisitesHTML(event.Prerequisites) +
    '<div class="subpanel"><h3>Изображение события</h3><label class="field"><span>Путь или URL (ImageURL)</span><input data-field="ImageURL" value="' + esc(event.ImageURL || '') + '" placeholder="images/event.png или https://…"></label>' +
      '<label class="field"><span>Загрузить файл (PNG, JPEG, WebP, GIF; до 10 МБ)</span><input id="event-image" type="file" accept="image/png,image/jpeg,image/webp,image/gif"></label>' + image + '</div>';
}
function renderEventList() {
  const query = ($('event-search')?.value || '').trim().toLocaleLowerCase();
  const events = arr(game.Events);
  $('event-list').innerHTML = events.map((event, index) => ({event, index})).filter(item => {
    const text = [item.event.ID, item.event.Name, item.event.Branch, item.event.Description].join(' ').toLocaleLowerCase();
    return text.includes(query);
  }).map(({event, index}) => '<button type="button" class="event-item ' + (index === selectedEventIndex ? 'active' : '') + '" data-select-event="' + index + '"><strong>' + Number(event.ID ?? 0) + '. ' + esc(event.Name || 'Без названия') + '</strong><small>' + esc(event.Branch || 'Без ветки') + ' · итерация ' + Number(event.Iteration ?? 0) + '</small></button>').join('');
  if (!events.length) $('event-list').innerHTML = '<p class="muted">Событий пока нет.</p>';
  else if (!$('event-list').innerHTML) $('event-list').innerHTML = '<p class="muted">Ничего не найдено.</p>';
}
function renderEventEditor() {
  $('event-editor').innerHTML = eventEditor(arr(game.Events)[selectedEventIndex]);
}
function eventSection() {
  return '<section class="panel"><div class="panel-head"><h2>События</h2><span class="muted">' + arr(game.Events).length + ' шт.</span></div><div class="columns">' +
    '<aside class="event-sidebar"><input id="event-search" type="search" placeholder="Поиск события…"><div id="event-list" class="event-list"></div><div class="event-actions"><button type="button" id="add-event">+ Событие</button><button type="button" id="duplicate-event" class="light">Копировать</button><button type="button" id="delete-event" class="danger">Удалить</button></div></aside>' +
    '<div id="event-editor" class="mini-card"></div></div></section>';
}
function attributeCard(item, index, configurable) {
  return '<article class="mini-card attribute-card" data-index="' + index + '" data-configurable="' + configurable + '"><h3>' + (configurable ? 'Настраиваемый атрибут' : 'Базовый атрибут') + '</h3>' +
    '<div class="field-grid"><label class="field"><span>ID</span><input data-field="ID" type="number" value="' + Number(item.ID ?? 0) + '"></label><label class="field"><span>Название</span><input data-field="Name" value="' + esc(item.Name) + '"></label></div>' +
    '<div class="field-grid"><label class="field"><span>Значение по умолчанию</span><input data-field="DefaultValue" type="number" step="any" value="' + Number(item.DefaultValue ?? 0) + '"></label><label class="field"><span>Минимум</span><input data-field="MinValue" type="number" step="any" value="' + Number(item.MinValue ?? 0) + '"></label><label class="field"><span>Максимум</span><input data-field="MaxValue" type="number" step="any" value="' + Number(item.MaxValue ?? 1) + '"></label>' +
    (configurable ? '<label class="field"><span>Стоимость единицы</span><input data-field="CostPerUnit" type="number" step="any" value="' + Number(item.CostPerUnit ?? 0) + '"></label>' : '') + '</div>' +
    '<label class="field"><span>Описание</span><textarea data-field="Description">' + esc(item.Description) + '</textarea></label>' +
    '<div class="event-actions"><button type="button" class="danger small-button" data-remove-attribute="' + (configurable ? 'configurable' : 'basic') + ':' + index + '">Удалить атрибут</button></div></article>';
}
function strategyEditor(item) {
  if (!item) return '<div class="empty">Добавьте первую стратегию, чтобы начать редактирование.</div>';
  const changes = arr(item.AttributeChanges);
  return '<div class="panel-head"><div><h2>Параметры стратегии</h2><span class="muted">Настройки выбранной стратегии</span></div><span class="tag">ID ' + Number(item.ID ?? 0) + '</span></div>' +
    '<div class="field-grid">' +
      '<label class="field"><span>ID</span><input data-field="ID" type="number" step="1" value="' + Number(item.ID ?? 0) + '"></label>' +
      '<label class="field"><span>Название</span><input data-field="Name" value="' + esc(item.Name) + '"></label>' +
      '<label class="field"><span>Стоимость поддержки</span><input data-field="Cost" type="number" step="1" value="' + Number(item.Cost ?? 0) + '"></label>' +
    '</div>' +
    '<label class="field"><span>Описание</span><textarea data-field="Description">' + esc(item.Description) + '</textarea></label>' +
    '<div class="subpanel"><h3>Изменения атрибутов при активации</h3><div class="row-list" id="strategy-changes">' + changes.map((change, i) => attributeChangeHTML(change, i, 'strategy')).join('') + '</div><button type="button" class="light small-button" id="add-strategy-change">+ Добавить изменение</button></div>' +
    prerequisitesHTML(item.Prerequisites);
}
function renderStrategyList() {
  const query = ($('strategy-search')?.value || '').trim().toLocaleLowerCase();
  const strategies = arr(game.Strategies);
  $('strategy-list').innerHTML = strategies.map((item, index) => ({item, index})).filter(({item}) => {
    return [item.ID, item.Name, item.Description].join(' ').toLocaleLowerCase().includes(query);
  }).map(({item, index}) => '<button type="button" class="event-item ' + (index === selectedStrategyIndex ? 'active' : '') + '" data-select-strategy="' + index + '"><strong>' + Number(item.ID ?? 0) + '. ' + esc(item.Name || 'Без названия') + '</strong><small>Стоимость: ' + Number(item.Cost ?? 0) + ' · изменений: ' + arr(item.AttributeChanges).length + '</small></button>').join('');
  if (!strategies.length) $('strategy-list').innerHTML = '<p class="muted">Стратегий пока нет.</p>';
  else if (!$('strategy-list').innerHTML) $('strategy-list').innerHTML = '<p class="muted">Ничего не найдено.</p>';
}
function renderStrategyEditor() {
  $('strategy-editor').innerHTML = strategyEditor(arr(game.Strategies)[selectedStrategyIndex]);
}
function strategySection() {
  return '<section class="panel"><div class="panel-head"><h2>Стратегии</h2><span class="muted">' + arr(game.Strategies).length + ' шт.</span></div><div class="columns">' +
    '<aside class="event-sidebar"><input id="strategy-search" type="search" placeholder="Поиск стратегии…"><div id="strategy-list" class="event-list"></div><div class="event-actions"><button type="button" id="add-strategy">+ Стратегия</button><button type="button" id="duplicate-strategy" class="light">Копировать</button><button type="button" id="delete-strategy" class="danger">Удалить</button></div></aside>' +
    '<div id="strategy-editor" class="mini-card"></div></div></section>';
}
function hintEditor(item) {
  if (!item) return '<div class="empty">Добавьте первую подсказку, чтобы начать редактирование.</div>';
  return '<div class="panel-head"><div><h2>Параметры подсказки</h2><span class="muted">Настройки выбранной подсказки</span></div><span class="tag">ID ' + Number(item.ID ?? 0) + '</span></div>' +
    '<div class="field-grid">' +
      '<label class="field"><span>ID</span><input data-field="ID" type="number" step="1" value="' + Number(item.ID ?? 0) + '"></label>' +
      '<label class="field"><span>Название</span><input data-field="Name" value="' + esc(item.Name) + '"></label>' +
      '<label class="field"><span>Стоимость (QP)</span><input data-field="Cost" type="number" step="1" value="' + Number(item.Cost ?? 0) + '"></label>' +
      '<label class="field"><span>Надёжность</span><input data-field="Reliability" value="' + esc(item.Reliability) + '"></label>' +
    '</div>' +
    '<label class="field"><span>Описание</span><textarea data-field="Description">' + esc(item.Description) + '</textarea></label>' +
    '<label class="field"><span>Секретная информация</span><textarea class="hint-secret" data-field="Secret">' + esc(item.Secret) + '</textarea></label>' +
    prerequisitesHTML(item.Prerequisites);
}
function renderHintList() {
  const query = ($('hint-search')?.value || '').trim().toLocaleLowerCase();
  const hints = arr(game.Hints);
  $('hint-list').innerHTML = hints.map((item, index) => ({item, index})).filter(({item}) => {
    return [item.ID, item.Name, item.Description, item.Reliability].join(' ').toLocaleLowerCase().includes(query);
  }).map(({item, index}) => '<button type="button" class="event-item ' + (index === selectedHintIndex ? 'active' : '') + '" data-select-hint="' + index + '"><strong>' + Number(item.ID ?? 0) + '. ' + esc(item.Name || 'Без названия') + '</strong><small>Стоимость: ' + Number(item.Cost ?? 0) + ' QP · ' + esc(item.Reliability || 'Надёжность не указана') + '</small></button>').join('');
  if (!hints.length) $('hint-list').innerHTML = '<p class="muted">Подсказок пока нет.</p>';
  else if (!$('hint-list').innerHTML) $('hint-list').innerHTML = '<p class="muted">Ничего не найдено.</p>';
}
function renderHintEditor() {
  $('hint-editor').innerHTML = hintEditor(arr(game.Hints)[selectedHintIndex]);
}
function hintSection() {
  return '<section class="panel"><div class="panel-head"><h2>Подсказки</h2><span class="muted">' + arr(game.Hints).length + ' шт.</span></div><div class="columns">' +
    '<aside class="event-sidebar"><input id="hint-search" type="search" placeholder="Поиск подсказки…"><div id="hint-list" class="event-list"></div><div class="event-actions"><button type="button" id="add-hint">+ Подсказка</button><button type="button" id="duplicate-hint" class="light">Копировать</button><button type="button" id="delete-hint" class="danger">Удалить</button></div></aside>' +
    '<div id="hint-editor" class="mini-card"></div></div></section>';
}
function render() {
  const budget = game.StartingBudget || {};
  editor.innerHTML =
    '<section class="panel"><h2>Настройки игры</h2><div class="field-grid">' +
      '<label class="field"><span>Ветки (через запятую)</span><input id="branches" value="' + esc(arr(game.Branches).join(', ')) + '"></label>' +
      '<label class="field"><span>ID стартового события</span><input id="opening-event" type="number" value="' + Number(game.OpeningEvent ?? 0) + '"></label>' +
      '<label class="field"><span>Минимальная сумма вероятностей (MinProbabilitySum)</span><input id="min-probability-sum" type="number" min="0" step="any" value="' + Number(game.MinProbabilitySum ?? 0) + '"></label>' +
      '<label class="field"><span>Начальные деньги</span><input id="initial-money" type="number" value="' + Number(budget.InitialMoney ?? 0) + '"></label>' +
      '<label class="field"><span>Максимум денег</span><input id="max-money" type="number" value="' + Number(budget.MaxMoney ?? 0) + '"></label>' +
      '<label class="field"><span>Начальные очки качества</span><input id="initial-qp" type="number" value="' + Number(budget.InitialQP ?? 0) + '"></label>' +
    '</div></section>' +
    '<section class="panel"><div class="panel-head"><h2>Базовые атрибуты</h2><button type="button" id="add-basic-attribute">+ Добавить базовый атрибут</button></div><div class="attribute-grid">' + arr(game.BasicAttributes).map((item, i) => attributeCard(item, i, false)).join('') + (arr(game.BasicAttributes).length ? '' : '<p class="muted">Базовые атрибуты ещё не добавлены.</p>') + '</div></section>' +
    '<section class="panel"><div class="panel-head"><h2>Конфигурируемые атрибуты</h2><button type="button" id="add-configurable-attribute">+ Добавить конфигурируемый атрибут</button></div><div class="attribute-grid">' + arr(game.ConfigurableAttributes).map((item, i) => attributeCard(item, i, true)).join('') + (arr(game.ConfigurableAttributes).length ? '' : '<p class="muted">Конфигурируемые атрибуты ещё не добавлены.</p>') + '</div></section>' +
    eventSection() +
    strategySection() +
    hintSection();
  renderEventList();
  renderEventEditor();
  renderStrategyList();
  renderStrategyEditor();
  renderHintList();
  renderHintEditor();
  bind();
}
function collectSetup() {
  game.Branches = ($('branches')?.value || '').split(',').map(value => value.trim()).filter(Boolean);
  game.OpeningEvent = Number($('opening-event')?.value || 0);
  game.MinProbabilitySum = Number($('min-probability-sum')?.value || 0);
  if (!Number.isFinite(game.MinProbabilitySum) || game.MinProbabilitySum < 0) {
    throw new Error('MinProbabilitySum должен быть конечным неотрицательным числом');
  }
  game.StartingBudget = game.StartingBudget || {};
  game.StartingBudget.InitialMoney = Number($('initial-money')?.value || 0);
  game.StartingBudget.MaxMoney = Number($('max-money')?.value || 0);
  game.StartingBudget.InitialQP = Number($('initial-qp')?.value || 0);
}
function collectAttributes() {
  const updates = [];
  const idMap = new Map();

  document.querySelectorAll('.attribute-card').forEach(card => {
    const index = Number(card.dataset.index);
    const configurable = card.dataset.configurable === 'true';
    const item = (configurable ? arr(game.ConfigurableAttributes) : arr(game.BasicAttributes))[index];
    if (!item) return;

    const fields = {};
    card.querySelectorAll('[data-field]').forEach(input => {
      const field = input.dataset.field;
      fields[field] = ['ID','DefaultValue','MinValue','MaxValue','CostPerUnit'].includes(field)
        ? Number(input.value)
        : input.value;
    });

    const oldID = Number(item.ID);
    const newID = Number(fields.ID);
    if (oldID !== newID) idMap.set(oldID, newID);
    updates.push({item, fields});
  });

  // Keep all references in sync when an attribute ID is edited. Weight
  // coordinates are positional, so their order does not change here.
  if (idMap.size) {
    const remap = id => idMap.has(Number(id)) ? idMap.get(Number(id)) : Number(id);
    arr(game.Events).forEach(event => {
      arr(event.Thresholds).forEach(threshold => { threshold.AttributeID = remap(threshold.AttributeID); });
      arr(event.AttributeChanges).forEach(change => { change.AttributeID = remap(change.AttributeID); });
    });
    arr(game.Strategies).forEach(strategy => {
      arr(strategy.AttributeChanges).forEach(change => { change.AttributeID = remap(change.AttributeID); });
    });
  }

  updates.forEach(({item, fields}) => Object.assign(item, fields));
}
function collectPrerequisites(card) {
  return {Events: valuesFrom(card.querySelector('.req-events')), Strategies: valuesFrom(card.querySelector('.req-strategies'))};
}
function collectAttributeChanges(container) {
  return [...container.querySelectorAll('.edit-row')].map(row => ({
    AttributeID: Number(row.querySelector('[data-change-field="AttributeID"]').value),
    AttributeShift: Number(row.querySelector('[data-change-field="AttributeShift"]').value)
  }));
}
function collectSelectedEvent() {
  const event = arr(game.Events)[selectedEventIndex];
  const card = $('event-editor');
  if (!event || !card) return;
  card.querySelectorAll('[data-field]').forEach(input => {
    const field = input.dataset.field;
    if (field === 'ID' || field === 'Iteration' || field === 'BasicPower') event[field] = Number(input.value);
    else if (field === 'IsPositive') event[field] = input.checked;
    else event[field] = input.value;
  });
  event.Weights = [...card.querySelectorAll('[data-weight-index]')].map(input => Number(input.value));
  event.AttributeChanges = collectAttributeChanges($('event-changes'));
  event.BudgetChange = {
    MoneyShift: Number(card.querySelector('[data-budget-field="MoneyShift"]').value),
    QPShift: Number(card.querySelector('[data-budget-field="QPShift"]').value)
  };
  const previousThresholds = arr(event.Thresholds);
  event.Thresholds = [...card.querySelectorAll('[data-threshold-field="AttributeID"]')].map((input, index) => {
    const row = input.closest('.edit-row');
    const previous = previousThresholds[index] || {};
    return {
      AttributeID: Number(input.value),
      Value: Number(previous.Value ?? 0),
      Min: Number(row.querySelector('[data-threshold-field="Min"]').value),
      Max: Number(row.querySelector('[data-threshold-field="Max"]').value)
    };
  });
  event.Prerequisites = collectPrerequisites(card);
}
function collectSelectedStrategy() {
  const item = arr(game.Strategies)[selectedStrategyIndex];
  const card = $('strategy-editor');
  if (!item || !card) return;
  card.querySelectorAll('[data-field]').forEach(input => {
    const field = input.dataset.field;
    item[field] = ['ID', 'Cost'].includes(field) ? Number(input.value) : input.value;
  });
  item.AttributeChanges = collectAttributeChanges(card);
  item.Prerequisites = collectPrerequisites(card);
}
function collectSelectedHint() {
  const item = arr(game.Hints)[selectedHintIndex];
  const card = $('hint-editor');
  if (!item || !card) return;
  card.querySelectorAll('[data-field]').forEach(input => {
    const field = input.dataset.field;
    item[field] = ['ID', 'Cost'].includes(field) ? Number(input.value) : input.value;
  });
  item.Prerequisites = collectPrerequisites(card);
}
function collect() {
  collectSetup();
  // Collect editor fields before remapping attribute IDs so references from
  // the currently visible event/strategy are updated together with the data.
  collectSelectedEvent();
  collectSelectedStrategy();
  collectSelectedHint();
  collectAttributes();
}
function nextID(items) {
  return arr(items).reduce((max, item) => Math.max(max, Number(item.ID) || 0), -1) + 1;
}
function firstAttributeID() {
  const attribute = allAttributes()[0];
  return attribute ? Number(attribute.ID) : 0;
}
function insertWeightSlot(index, previousAttributeCount) {
  arr(game.Events).forEach(event => {
    const weights = arr(event.Weights).slice();
    while (weights.length < previousAttributeCount) weights.push(0);
    weights.splice(Math.min(index, weights.length), 0, 0);
    event.Weights = weights;
  });
}
function removeAttributeReferences(attributeID) {
  arr(game.Events).forEach(event => {
    event.Thresholds = arr(event.Thresholds).filter(item => Number(item.AttributeID) !== attributeID);
    event.AttributeChanges = arr(event.AttributeChanges).filter(item => Number(item.AttributeID) !== attributeID);
  });
  arr(game.Strategies).forEach(strategy => {
    strategy.AttributeChanges = arr(strategy.AttributeChanges).filter(item => Number(item.AttributeID) !== attributeID);
  });
}
function addAttribute(configurable) {
  collect();
  game.BasicAttributes = arr(game.BasicAttributes);
  game.ConfigurableAttributes = arr(game.ConfigurableAttributes);
  const previousBasicCount = game.BasicAttributes.length;
  const previousAttributeCount = previousBasicCount + game.ConfigurableAttributes.length;
  const attribute = {
    ID: nextID(allAttributes()),
    Name: configurable ? 'Новый конфигурируемый атрибут' : 'Новый базовый атрибут',
    Description: '',
    DefaultValue: 0,
    MinValue: 0,
    MaxValue: 1
  };
  let weightIndex;
  if (configurable) {
    attribute.CostPerUnit = 0;
    weightIndex = previousAttributeCount;
    game.ConfigurableAttributes.push(attribute);
  } else {
    weightIndex = previousBasicCount;
    game.BasicAttributes.push(attribute);
  }
  insertWeightSlot(weightIndex, previousAttributeCount);
  render();
  markDirty();
}
function deleteAttribute(kind, index) {
  collect();
  const list = kind === 'configurable' ? game.ConfigurableAttributes : game.BasicAttributes;
  const item = arr(list)[index];
  if (!item) return;
  const label = item.Name || ('атрибут #' + item.ID);
  if (!confirm('Удалить «' + label + '»? Все связанные пороги и изменения этого атрибута также будут удалены.')) return;
  const attributeID = Number(item.ID);
  const weightIndex = kind === 'configurable' ? game.BasicAttributes.length + index : index;
  list.splice(index, 1);
  arr(game.Events).forEach(event => {
    const weights = arr(event.Weights).slice();
    if (weightIndex < weights.length) weights.splice(weightIndex, 1);
    event.Weights = weights;
  });
  removeAttributeReferences(attributeID);
  render();
  markDirty();
}
function bind() {
  $('add-basic-attribute').onclick = () => {
    try { addAttribute(false); } catch (error) { setStatus(error.message, 'error'); }
  };
  $('add-configurable-attribute').onclick = () => {
    try { addAttribute(true); } catch (error) { setStatus(error.message, 'error'); }
  };
  document.querySelectorAll('[data-remove-attribute]').forEach(button => button.addEventListener('click', () => {
    try {
      const [kind, index] = button.dataset.removeAttribute.split(':');
      deleteAttribute(kind, Number(index));
    } catch (error) { setStatus(error.message, 'error'); }
  }));
  $('event-search').addEventListener('input', renderEventList);
  $('event-list').addEventListener('click', event => {
    const button = event.target.closest('[data-select-event]');
    if (!button) return;
    try {
      collectSelectedEvent();
      selectedEventIndex = Number(button.dataset.selectEvent);
      renderEventList();
      renderEventEditor();
    } catch (error) { setStatus(error.message, 'error'); }
  });
  $('add-event').onclick = () => {
    try {
      collect();
      const event = {
        ID: nextID(game.Events), Name: 'Новое событие', Description: '', ImageURL: '', IsPositive: false,
        Weights: allAttributes().map(() => 0), BasicPower: 0, Branch: arr(game.Branches)[0] || '', Iteration: 0,
        Prerequisites: {Events: [], Strategies: []}, Thresholds: [], AttributeChanges: [], BudgetChange: {MoneyShift: 0, QPShift: 0}
      };
      game.Events = arr(game.Events);
      game.Events.push(event);
      selectedEventIndex = game.Events.length - 1;
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  $('duplicate-event').onclick = () => {
    try {
      collect();
      const current = arr(game.Events)[selectedEventIndex];
      if (!current) return;
      const copy = JSON.parse(JSON.stringify(current));
      copy.ID = nextID(game.Events);
      copy.Name = (copy.Name || 'Событие') + ' — копия';
      game.Events.push(copy);
      selectedEventIndex = game.Events.length - 1;
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  $('delete-event').onclick = () => {
    try {
      collect();
      const current = arr(game.Events)[selectedEventIndex];
      if (!current || !confirm('Удалить событие «' + (current.Name || current.ID) + '»?')) return;
      const removedID = Number(current.ID);
      game.Events.splice(selectedEventIndex, 1);
      arr(game.Events).forEach(item => { item.Prerequisites = item.Prerequisites || {}; item.Prerequisites.Events = arr(item.Prerequisites.Events).filter(id => Number(id) !== removedID); });
      arr(game.Strategies).forEach(item => { item.Prerequisites = item.Prerequisites || {}; item.Prerequisites.Events = arr(item.Prerequisites.Events).filter(id => Number(id) !== removedID); });
      arr(game.Hints).forEach(item => { item.Prerequisites = item.Prerequisites || {}; item.Prerequisites.Events = arr(item.Prerequisites.Events).filter(id => Number(id) !== removedID); });
      if (Number(game.OpeningEvent) === removedID) game.OpeningEvent = game.Events.length ? Number(game.Events[0].ID) : 0;
      selectedEventIndex = Math.max(0, Math.min(selectedEventIndex, game.Events.length - 1));
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  // Delegate clicks from the persistent editor container. Its inner HTML is
  // replaced whenever the selected event changes or a row is added/removed.
  $('event-editor').addEventListener('click', event => {
    const addThreshold = event.target.closest('#add-threshold');
    const addChange = event.target.closest('#add-event-change');
    const threshold = event.target.closest('[data-remove-threshold]');
    const change = event.target.closest('[data-remove-change]');

    try {
      if (addThreshold) {
        event.preventDefault();
        if (!allAttributes().length) {
          setStatus('Сначала добавьте хотя бы один атрибут.', 'error');
          return;
        }
        collectSelectedEvent();
        const selectedEvent = arr(game.Events)[selectedEventIndex];
        if (!selectedEvent) throw new Error('Сначала выберите событие.');
        selectedEvent.Thresholds = arr(selectedEvent.Thresholds);
        const attribute = allAttributes()[0];
        selectedEvent.Thresholds.push({
          AttributeID: Number(attribute.ID),
          Value: Number(attribute.DefaultValue ?? 0),
          Min: Number(attribute.MinValue ?? 0),
          Max: Number(attribute.MaxValue ?? 0)
        });
        renderEventEditor();
        markDirty();
        setStatus('Порог добавлен. Не забудьте сохранить сценарий.', 'dirty');
        return;
      }

      if (addChange) {
        event.preventDefault();
        if (!allAttributes().length) {
          setStatus('Сначала добавьте хотя бы один атрибут.', 'error');
          return;
        }
        collectSelectedEvent();
        const selectedEvent = arr(game.Events)[selectedEventIndex];
        if (!selectedEvent) throw new Error('Сначала выберите событие.');
        selectedEvent.AttributeChanges = arr(selectedEvent.AttributeChanges);
        selectedEvent.AttributeChanges.push({AttributeID: firstAttributeID(), AttributeShift: 0});
        renderEventEditor();
        markDirty();
        setStatus('Изменение добавлено. Не забудьте сохранить сценарий.', 'dirty');
        return;
      }

      if (threshold) {
        event.preventDefault();
        collectSelectedEvent();
        const selectedEvent = arr(game.Events)[selectedEventIndex];
        if (!selectedEvent) throw new Error('Сначала выберите событие.');
        selectedEvent.Thresholds = arr(selectedEvent.Thresholds);
        selectedEvent.Thresholds.splice(Number(threshold.dataset.removeThreshold), 1);
        renderEventEditor();
        markDirty();
        return;
      }

      if (change && change.dataset.removeChange.startsWith('event:')) {
        event.preventDefault();
        collectSelectedEvent();
        const selectedEvent = arr(game.Events)[selectedEventIndex];
        if (!selectedEvent) throw new Error('Сначала выберите событие.');
        selectedEvent.AttributeChanges = arr(selectedEvent.AttributeChanges);
        selectedEvent.AttributeChanges.splice(Number(change.dataset.removeChange.split(':')[1]), 1);
        renderEventEditor();
        markDirty();
      }
    } catch (error) {
      setStatus(error.message || 'Не удалось изменить событие.', 'error');
    }
  });
  $('event-editor').addEventListener('change', async event => {
    if (event.target.id !== 'event-image') return;
    const file = event.target.files[0];
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) { setStatus('Файл превышает 10 МБ', 'error'); return; }
    const targetIndex = selectedEventIndex;
    try {
      collectSelectedEvent();
      const data = new FormData();
      data.append('image', file);
      setStatus('Загрузка изображения…');
      const result = await request('/api/image?name=' + encodeURIComponent(scenarioName), {method: 'POST', body: data});
      game.Events[targetIndex].ImageURL = result.imageUrl;
      if (selectedEventIndex === targetIndex) renderEventEditor();
      markDirty();
      setStatus('Изображение загружено; сохраните сценарий', 'dirty');
    } catch (error) { setStatus(error.message, 'error'); }
  });
  $('strategy-search').addEventListener('input', renderStrategyList);
  $('strategy-list').addEventListener('click', event => {
    const button = event.target.closest('[data-select-strategy]');
    if (!button) return;
    try {
      collectSelectedStrategy();
      selectedStrategyIndex = Number(button.dataset.selectStrategy);
      renderStrategyList();
      renderStrategyEditor();
    } catch (error) { setStatus(error.message, 'error'); }
  });
  $('strategy-editor').addEventListener('click', event => {
    const addChange = event.target.closest('#add-strategy-change');
    const removeChange = event.target.closest('[data-remove-change]');
    try {
      if (addChange) {
        event.preventDefault();
        if (!allAttributes().length) {
          setStatus('Сначала добавьте хотя бы один атрибут.', 'error');
          return;
        }
        collectSelectedStrategy();
        const strategy = arr(game.Strategies)[selectedStrategyIndex];
        if (!strategy) throw new Error('Сначала выберите стратегию.');
        strategy.AttributeChanges = arr(strategy.AttributeChanges);
        strategy.AttributeChanges.push({AttributeID: firstAttributeID(), AttributeShift: 0});
        renderStrategyEditor();
        renderStrategyList();
        markDirty();
        return;
      }
      if (removeChange && removeChange.dataset.removeChange.startsWith('strategy:')) {
        event.preventDefault();
        collectSelectedStrategy();
        const strategy = arr(game.Strategies)[selectedStrategyIndex];
        if (!strategy) throw new Error('Сначала выберите стратегию.');
        strategy.AttributeChanges = arr(strategy.AttributeChanges);
        strategy.AttributeChanges.splice(Number(removeChange.dataset.removeChange.split(':')[1]), 1);
        renderStrategyEditor();
        renderStrategyList();
        markDirty();
      }
    } catch (error) { setStatus(error.message || 'Не удалось изменить стратегию.', 'error'); }
  });
  $('add-strategy').onclick = () => {
    try {
      collect();
      game.Strategies.push({ID: nextID(game.Strategies), Name: 'Новая стратегия', Description: '', Cost: 0, Prerequisites: {Events: [], Strategies: []}, AttributeChanges: []});
      selectedStrategyIndex = game.Strategies.length - 1;
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  $('duplicate-strategy').onclick = () => {
    try {
      collect();
      const current = arr(game.Strategies)[selectedStrategyIndex];
      if (!current) return;
      const copy = JSON.parse(JSON.stringify(current));
      copy.ID = nextID(game.Strategies);
      copy.Name = (copy.Name || 'Стратегия') + ' — копия';
      game.Strategies.push(copy);
      selectedStrategyIndex = game.Strategies.length - 1;
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  $('delete-strategy').onclick = () => {
    try {
      collect();
      const current = arr(game.Strategies)[selectedStrategyIndex];
      if (!current || !confirm('Удалить стратегию «' + (current.Name || current.ID) + '»?')) return;
      const removedID = Number(current.ID);
      game.Strategies.splice(selectedStrategyIndex, 1);
      arr(game.Events).forEach(item => { item.Prerequisites = item.Prerequisites || {}; item.Prerequisites.Strategies = arr(item.Prerequisites.Strategies).filter(id => Number(id) !== removedID); });
      arr(game.Strategies).forEach(item => { item.Prerequisites = item.Prerequisites || {}; item.Prerequisites.Strategies = arr(item.Prerequisites.Strategies).filter(id => Number(id) !== removedID); });
      arr(game.Hints).forEach(item => { item.Prerequisites = item.Prerequisites || {}; item.Prerequisites.Strategies = arr(item.Prerequisites.Strategies).filter(id => Number(id) !== removedID); });
      selectedStrategyIndex = Math.max(0, Math.min(selectedStrategyIndex, game.Strategies.length - 1));
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };

  $('hint-search').addEventListener('input', renderHintList);
  $('hint-list').addEventListener('click', event => {
    const button = event.target.closest('[data-select-hint]');
    if (!button) return;
    try {
      collectSelectedHint();
      selectedHintIndex = Number(button.dataset.selectHint);
      renderHintList();
      renderHintEditor();
    } catch (error) { setStatus(error.message, 'error'); }
  });
  $('add-hint').onclick = () => {
    try {
      collect();
      game.Hints.push({ID: nextID(game.Hints), Name: 'Новая подсказка', Description: '', Cost: 0, Reliability: '', Secret: '', Prerequisites: {Events: [], Strategies: []}});
      selectedHintIndex = game.Hints.length - 1;
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  $('duplicate-hint').onclick = () => {
    try {
      collect();
      const current = arr(game.Hints)[selectedHintIndex];
      if (!current) return;
      const copy = JSON.parse(JSON.stringify(current));
      copy.ID = nextID(game.Hints);
      copy.Name = (copy.Name || 'Подсказка') + ' — копия';
      game.Hints.push(copy);
      selectedHintIndex = game.Hints.length - 1;
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
  $('delete-hint').onclick = () => {
    try {
      collect();
      const current = arr(game.Hints)[selectedHintIndex];
      if (!current || !confirm('Удалить подсказку «' + (current.Name || current.ID) + '»?')) return;
      game.Hints.splice(selectedHintIndex, 1);
      selectedHintIndex = Math.max(0, Math.min(selectedHintIndex, game.Hints.length - 1));
      render();
      markDirty();
    } catch (error) { setStatus(error.message, 'error'); }
  };
}
async function loadScenario(force = false) {
  const requestedName = select.value;
  if (dirty && !force && !confirm('Есть несохранённые изменения. Загрузить другой сценарий и отбросить их?')) {
    select.value = scenarioName;
    return;
  }
  if (!requestedName) { editor.innerHTML = '<div class="empty">В папке scenarios нет JSON-сценариев.</div>'; return; }
  try {
    const response = await request('/api/scenario?name=' + encodeURIComponent(requestedName));
    scenarioName = requestedName;
    game = response.game || {};
    game.Events = arr(game.Events);
    game.Strategies = arr(game.Strategies);
    game.Hints = arr(game.Hints);
    game.BasicAttributes = arr(game.BasicAttributes);
    game.ConfigurableAttributes = arr(game.ConfigurableAttributes);
    selectedEventIndex = 0;
    selectedStrategyIndex = 0;
    selectedHintIndex = 0;
    dirty = false;
    render();
    setStatus('Загружен: ' + scenarioName);
  } catch (error) {
    setStatus(error.message, 'error');
    editor.innerHTML = '<div class="empty">Не удалось загрузить сценарий.</div>';
  }
}
async function save() {
  if (!scenarioName) { setStatus('Сначала создайте сценарий', 'error'); return; }
  try {
    collect();
    await request('/api/scenario?name=' + encodeURIComponent(scenarioName), {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name: scenarioName, game})
    });
    dirty = false;
    setStatus('Сохранено: ' + scenarioName);
  } catch (error) { setStatus(error.message, 'error'); }
}
async function refreshScenarioList(selectedName = '') {
  const names = await request('/api/scenarios');
  select.innerHTML = names.map(name => '<option value="' + esc(name) + '">' + esc(name) + '</option>').join('');
  if (selectedName) select.value = selectedName;
  return names;
}
async function createScenario() {
  if (dirty && !confirm('Есть несохранённые изменения. Создать новый сценарий и оставить текущие изменения несохранёнными?')) return;
  const enteredName = prompt('Введите имя нового сценария (без расширения .json):');
  if (enteredName === null) return;
  const name = enteredName.trim();
  if (!name) { setStatus('Имя сценария не может быть пустым', 'error'); return; }
  try {
    await request('/api/scenario?name=' + encodeURIComponent(name), {method: 'POST'});
    await refreshScenarioList(name);
    await loadScenario(true);
    setStatus('Создан сценарий: ' + name);
  } catch (error) { setStatus(error.message, 'error'); }
}
async function init() {
  try {
    const names = await refreshScenarioList();
    if (names.length) await loadScenario(true);
    else { editor.innerHTML = '<div class="empty">Пока нет сценариев. Нажмите «Новый сценарий», чтобы создать пустой.</div>'; setStatus('Нет сценариев'); }
  } catch (error) { setStatus(error.message, 'error'); }
}
editor.addEventListener('input', event => { if (!['event-search', 'strategy-search', 'hint-search'].includes(event.target.id)) markDirty(); });
editor.addEventListener('change', event => { if (!['event-search', 'strategy-search', 'hint-search', 'event-image'].includes(event.target.id)) markDirty(); });
select.onchange = () => loadScenario();
$('new-scenario').onclick = createScenario;
$('reload').onclick = () => loadScenario();
$('save').onclick = save;
window.addEventListener('beforeunload', event => { if (dirty) { event.preventDefault(); event.returnValue = ''; } });
init();
</script>
</body>
</html>`
