package renderer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/leekchan/gtf"
	"go.uber.org/zap"

	"github.com/vgarvardt/rklotz/pkg/server/plugin"
	"github.com/vgarvardt/rklotz/pkg/server/rqctx"
)

// HTMLConfig is configuration for HTML renderer
type HTMLConfig struct {
	Debug         bool
	TemplatesPath string
	UICfg         UIConfig
	RootURLCfg    RootURLConfig
	PluginsCfg    plugin.Config
}

// HTML implements Renderer for HTML content
type HTML struct {
	templates  map[string]*template.Template
	config     HTMLConfig
	logger     *zap.Logger
	instanceID string

	enabledPluginsMap map[string]bool
	pluginsSettings   map[string]map[string]template.JS
}

// NewHTML creates new HTML instance
func NewHTML(config HTMLConfig, logger *zap.Logger) (*HTML, error) {
	instance := &HTML{
		templates: make(map[string]*template.Template),
		config:    config,
		logger:    logger,
	}

	return instance, instance.initTemplates()
}

func (r *HTML) newID() string {
	hasher := md5.New()
	hasher.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return hex.EncodeToString(hasher.Sum(nil))[:6]
}

func (r *HTML) initTemplates() error {
	r.instanceID = r.newID()

	partials, err := r.getPartials(r.config.TemplatesPath, r.config.UICfg.Theme, r.config.UICfg.AboutPath)
	if nil != err {
		return err
	}

	baseFiles := append(partials, fmt.Sprintf("%s/%s/base.html", r.config.TemplatesPath, r.config.UICfg.Theme))
	baseTemplate := template.Must(
		template.New("base.html").Funcs(getTmplFuncMap(r.config.UICfg.DateFormat)).ParseFiles(baseFiles...))

	for _, tmplName := range []string{"index.html", "post.html", "tag.html"} {
		tmplPath := fmt.Sprintf("%s/%s/%s", r.config.TemplatesPath, r.config.UICfg.Theme, tmplName)

		r.logger.Debug("Initializing template", zap.String("name", tmplName), zap.String("path", tmplPath))
		r.templates[tmplName] = template.Must(template.Must(baseTemplate.Clone()).ParseFiles(tmplPath))
	}

	err = r.initPlugins()
	if err != nil {
		return err
	}

	return nil
}

func (r *HTML) getPartials(templatesPath, theme, uiAbout string) ([]string, error) {
	var partials []string

	walkFn := func(path string, f os.FileInfo, err error) error {
		if nil == err && !f.IsDir() && !strings.HasSuffix(path, "about.html") {
			partials = append(partials, path)
		}
		return err
	}

	pluginsPath := path.Join(templatesPath, "plugins")
	err := filepath.Walk(pluginsPath, walkFn)
	if err != nil {
		return nil, err
	}

	themePartialsPath := path.Join(templatesPath, theme, "partial")
	err = filepath.Walk(themePartialsPath, walkFn)
	if err != nil {
		return nil, err
	}

	_, err = os.Stat(uiAbout)
	if os.IsNotExist(err) {
		r.logger.Info("Custom about panel not found, loading default theme about panel", zap.String("path", uiAbout))
		uiAbout = fmt.Sprintf("%s/%s/partial/about.html", templatesPath, theme)
	} else if nil != err {
		r.logger.Error("Failed to load custom about panel", zap.Error(err), zap.String("path", uiAbout))
		return nil, err
	} else {
		r.logger.Info("Loading custom about panel", zap.String("path", uiAbout))
	}

	partials = append(partials, uiAbout)

	return partials, nil
}

func (r *HTML) initPlugins() error {
	r.enabledPluginsMap = make(map[string]bool, len(r.config.PluginsCfg.Enabled))
	r.pluginsSettings = make(map[string]map[string]template.JS, len(r.config.PluginsCfg.Enabled))

	for i := range r.config.PluginsCfg.Enabled {
		r.enabledPluginsMap[r.config.PluginsCfg.Enabled[i]] = true

		r.logger.Info("Loading plugin", zap.String("name", r.config.PluginsCfg.Enabled[i]))
		p, err := plugin.GetByName(r.config.PluginsCfg.Enabled[i])
		if err != nil {
			return err
		}

		r.logger.Info("Configuring plugin", zap.String("name", r.config.PluginsCfg.Enabled[i]))
		settings, err := r.config.PluginsCfg.SetUp(p)
		if err != nil {
			switch e := err.(type) {
			case *plugin.ErrorConfiguring:
				r.logger.Error("Failed to configure plugin", zap.Error(err), zap.String("field", e.Field()))
			}
			return err
		}

		r.pluginsSettings[r.config.PluginsCfg.Enabled[i]] = make(map[string]template.JS)
		for settingName, settingValue := range settings {
			r.pluginsSettings[r.config.PluginsCfg.Enabled[i]][settingName] = template.JS(settingValue)
		}
	}

	return nil
}

// Render renders the data with response code to a HTTP response writer
func (r *HTML) Render(w http.ResponseWriter, code int, data *Data) {
	data.m.RLock()
	defer data.m.RUnlock()

	if r.config.Debug {
		rqctx.GetLogger(data.r.Context()).Warn("HTML renderer is in the debug mode, reloading all templates")
		if err := r.initTemplates(); err != nil {
			rqctx.GetLogger(data.r.Context()).Error(
				"Problems with reloading HTML templates",
				zap.Error(err),
			)
			return
		}
	}

	templateData := data.data

	templateData["lang"] = r.config.UICfg.Language
	templateData["title"] = r.config.UICfg.Title
	templateData["heading"] = r.config.UICfg.Heading
	templateData["intro"] = r.config.UICfg.Intro
	templateData["theme"] = r.config.UICfg.Theme
	templateData["author"] = r.config.UICfg.Author
	templateData["description"] = r.config.UICfg.Description
	templateData["date_format"] = r.config.UICfg.DateFormat

	templateData["instance_id"] = r.instanceID
	templateData["plugins"] = r.enabledPluginsMap
	templateData["plugin"] = r.pluginsSettings

	templateData["url_path"] = data.r.URL.Path
	templateData["root_url"] = r.config.RootURLCfg.URL(data.r).String()

	currentURL := &url.URL{}
	*currentURL = *r.config.RootURLCfg.URL(data.r)
	currentURL.Path = data.r.URL.Path
	templateData["current_url"] = currentURL.String()

	logger := rqctx.GetLogger(data.r.Context())

	tmpl, found := r.templates[data.template]
	if !found {
		logger.Error("Template is not found in the templates registry", zap.String("template", data.template))
		panic(fmt.Errorf("template is not found in the templates registry %q", data.template))
	}

	w.WriteHeader(code)

	err := tmpl.Execute(w, templateData)
	if nil != err {
		logger.Error(
			"Problems with rendering HTML template",
			zap.Error(err),
			zap.String("template", data.template),
		)
	}
}

func getTmplFuncMap(dateFormat string) template.FuncMap {
	funcs := gtf.GtfFuncMap

	funcs["format_date"] = func(value time.Time) string {
		return value.Format(dateFormat)
	}
	funcs["add"] = func(arg int, value int) int {
		return value + arg
	}
	funcs["noescape"] = func(value string) template.HTML {
		return template.HTML(value)
	}

	return funcs
}
