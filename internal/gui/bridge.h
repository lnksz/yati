#ifndef YATI_GUI_BRIDGE_H
#define YATI_GUI_BRIDGE_H

enum {
    YATI_STARTUP = 1, YATI_ACTIVATE, YATI_SEARCH, YATI_START_NEW,
    YATI_RESUME, YATI_STOP, YATI_CONTINUE, YATI_REFRESH, YATI_QUIT,
    YATI_RESULT, YATI_TICK, YATI_PERIOD, YATI_PAGE, YATI_CONNECT,
    YATI_CONNECTION, YATI_AUTOSTART, YATI_SHORTCUT, YATI_HIDE, YATI_HELP,
    YATI_EDIT, YATI_UPDATE
};

int yati_run(int argc, char **argv);
void yati_post(int action, const char *text);
void yati_present(const char *token);
void yati_hide(void);
void yati_quit(void);
void yati_set_current(const char *title, const char *subtitle, const char *elapsed, int running, int continuable, int busy);
void yati_set_status(const char *text, int error, int busy);
void yati_set_connection(int visible);
void yati_set_connected(int connected);
void yati_set_autostart(int enabled);
void yati_set_query(const char *text);
const char *yati_query(void);
const char *yati_token(void);
int yati_remember(void);
int yati_project(void);
void yati_projects_begin(void);
void yati_project_add(const char *name);
void yati_projects_end(int selected);
void yati_editor_begin(void);
void yati_editor_project_add(const char *name);
void yati_editor_show(int selected, const char *title);
int yati_editor_visible(void);
void yati_list_begin(int history);
void yati_list_add(int history, int index, const char *title, const char *subtitle, const char *detail);
void yati_list_update_detail(int history, int index, const char *detail);
const char *yati_list_detail(int history, int index);
void yati_list_end(int history, const char *empty);
void yati_set_summary(const char *text);
void yati_toast(const char *text);
int yati_width(void);
int yati_height(void);
int yati_transitioning(void);
void yati_clear_token(void);
void yati_show_shortcuts(void);
void yati_set_icon(int running);
int yati_help_visible(void);

#endif
