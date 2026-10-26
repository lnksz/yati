//go:build linux && cgo && !nogui

#include "bridge.h"
#include <adwaita.h>
#include <stdint.h>

// Only this adapter owns GTK objects. Go owns application state and I/O.
extern void yati_go_event(int action, int index, char *text);

static AdwApplication *app;
static GtkWindow *window;
static GtkWindow *shortcuts_window;
static GtkWidget *app_menu;
static AdwToastOverlay *toasts;
static GtkWidget *search, *start_button, *stop_button, *continue_button;
static GtkWidget *current_row, *elapsed_label, *status_label, *spinner;
static GtkWidget *error_revealer, *error_label, *main_stack, *project_dropdown;
static GtkWidget *recent_list, *history_list, *summary_label;
static GtkWidget *token_entry, *remember_check, *connect_button, *autostart_check;
static GtkStringList *project_names;
static gboolean rendering = FALSE, connected = FALSE, working = FALSE;
static guint clock_source = 0;

static void event(int action, int index, const char *text) {
    yati_go_event(action, index, (char *)(text ? text : ""));
}

static GtkWidget *label(const char *text, const char *style) {
    GtkWidget *w = gtk_label_new(text);
    gtk_label_set_xalign(GTK_LABEL(w), 0);
    if (style) gtk_widget_add_css_class(w, style);
    return w;
}

static void clicked(GtkButton *button, gpointer data) {
    GtkWidget *popover = gtk_widget_get_ancestor(GTK_WIDGET(button), GTK_TYPE_POPOVER);
    if (popover) gtk_popover_popdown(GTK_POPOVER(popover));
    event(GPOINTER_TO_INT(data), 0, "");
}

static GtkWidget *button(const char *text, const char *icon, int action) {
    GtkWidget *w = gtk_button_new();
    if (text && icon) {
        GtkWidget *box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
        gtk_box_append(GTK_BOX(box), gtk_image_new_from_icon_name(icon));
        gtk_box_append(GTK_BOX(box), gtk_label_new(text));
        gtk_button_set_child(GTK_BUTTON(w), box);
    } else if (text) {
        gtk_button_set_label(GTK_BUTTON(w), text);
    } else {
        gtk_button_set_icon_name(GTK_BUTTON(w), icon);
    }
    g_signal_connect(w, "clicked", G_CALLBACK(clicked), GINT_TO_POINTER(action));
    return w;
}

static gboolean tick(gpointer data) {
    (void)data;
    event(YATI_TICK, 0, "");
    return G_SOURCE_CONTINUE;
}

void yati_hide(void) {
    if (clock_source) { g_source_remove(clock_source); clock_source = 0; }
    if (shortcuts_window) gtk_widget_set_visible(GTK_WIDGET(shortcuts_window), FALSE);
    gtk_widget_set_visible(GTK_WIDGET(window), FALSE);
}

static gboolean close_window(GtkWindow *w, gpointer data) {
    (void)w; (void)data;
    yati_hide();
    return TRUE;
}

static void search_changed(GtkEditable *editable, gpointer data) {
    (void)data;
    if (!rendering) event(YATI_SEARCH, 0, gtk_editable_get_text(editable));
    gtk_widget_set_sensitive(start_button, connected && !working && *gtk_editable_get_text(editable));
}

static void token_changed(GtkEditable *editable, gpointer data) {
    (void)data;
    gtk_widget_set_sensitive(connect_button, !working && *gtk_editable_get_text(editable));
}

static void row_activated(GtkListBox *list, GtkListBoxRow *row, gpointer data) {
    (void)list;
    if (!working) event(YATI_RESUME, GPOINTER_TO_INT(g_object_get_data(G_OBJECT(row), "entry-index")), data ? "history" : "recent");
}

static gboolean timer_shortcut(guint key, GdkModifierType modifiers) {
    if ((modifiers & GDK_CONTROL_MASK) && (key == GDK_KEY_s || key == GDK_KEY_S)) {
        event(YATI_STOP, 0, ""); return TRUE;
    }
    if ((modifiers & (GDK_CONTROL_MASK | GDK_SHIFT_MASK)) == (GDK_CONTROL_MASK | GDK_SHIFT_MASK) &&
        (key == GDK_KEY_c || key == GDK_KEY_C)) {
        event(YATI_CONTINUE, 0, ""); return TRUE;
    }
    return FALSE;
}

static gboolean shortcuts_key(GtkEventControllerKey *controller, guint key, guint code, GdkModifierType modifiers, gpointer data) {
    (void)controller; (void)code; (void)data;
    if (key == GDK_KEY_Escape) { gtk_widget_set_visible(GTK_WIDGET(shortcuts_window), FALSE); return TRUE; }
    if (timer_shortcut(key, modifiers)) return TRUE;
    if ((modifiers & GDK_CONTROL_MASK) && (key == GDK_KEY_q || key == GDK_KEY_Q)) {
        event(YATI_QUIT, 0, ""); return TRUE;
    }
    return FALSE;
}

static GtkWidget *shortcut_row(AdwPreferencesGroup *group, const char *title, const char *accelerator) {
    GtkWidget *row = adw_action_row_new();
    adw_preferences_row_set_title(ADW_PREFERENCES_ROW(row), title);
    adw_preferences_row_set_use_markup(ADW_PREFERENCES_ROW(row), FALSE);
    adw_action_row_set_title_lines(ADW_ACTION_ROW(row), 2);
    adw_action_row_add_suffix(ADW_ACTION_ROW(row), gtk_shortcut_label_new(accelerator));
    adw_preferences_group_add(group, row);
    return row;
}

void yati_show_shortcuts(void) {
    if (!shortcuts_window) {
        shortcuts_window = GTK_WINDOW(adw_window_new());
        gtk_window_set_title(shortcuts_window, "Keyboard shortcuts");
        gtk_window_set_transient_for(shortcuts_window, window);
        gtk_window_set_application(shortcuts_window, GTK_APPLICATION(app));
        gtk_window_set_modal(shortcuts_window, TRUE);
        gtk_window_set_hide_on_close(shortcuts_window, TRUE);
        gtk_window_set_default_size(shortcuts_window, 540, 650);
        GtkWidget *content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
        adw_window_set_content(ADW_WINDOW(shortcuts_window), content);
        GtkWidget *header = adw_header_bar_new();
        adw_header_bar_set_title_widget(ADW_HEADER_BAR(header), adw_window_title_new("Keyboard shortcuts", NULL));
        gtk_box_append(GTK_BOX(content), header);
        AdwPreferencesPage *page = ADW_PREFERENCES_PAGE(adw_preferences_page_new());
        gtk_widget_set_vexpand(GTK_WIDGET(page), TRUE);
        gtk_box_append(GTK_BOX(content), GTK_WIDGET(page));
        AdwPreferencesGroup *timer = ADW_PREFERENCES_GROUP(adw_preferences_group_new());
        adw_preferences_group_set_title(timer, "Timer");
        adw_preferences_group_set_description(timer, "With Yati focused. The desktop shortcut opens Yati from other apps.");
        adw_preferences_page_add(page, timer);
        shortcut_row(timer, "Stop current timer", "<Control>s");
        shortcut_row(timer, "Continue last stopped entry", "<Control><Shift>c");
        shortcut_row(timer, "Continue selected task", "Return");
        shortcut_row(timer, "Start a new task", "<Control>Return");
        shortcut_row(timer, "Select a result", "Up Down");
        AdwPreferencesGroup *navigation = ADW_PREFERENCES_GROUP(adw_preferences_group_new());
        adw_preferences_group_set_title(navigation, "Navigation");
        adw_preferences_page_add(page, navigation);
        shortcut_row(navigation, "Focus search", "<Control>l");
        shortcut_row(navigation, "Quick entry", "<Control>1");
        shortcut_row(navigation, "History", "<Control>2");
        shortcut_row(navigation, "Refresh from Toggl", "<Control>r");
        GtkWidget *help = shortcut_row(navigation, "Show keyboard shortcuts", "question <Control>question");
        adw_action_row_set_subtitle(ADW_ACTION_ROW(help), "Use ? with an empty search; Ctrl+? works while typing.");
        shortcut_row(navigation, "Open app menu", "F10");
        shortcut_row(navigation, "Hide Yati / close this window", "Escape");
        shortcut_row(navigation, "Quit Yati", "<Control>q");
        GtkEventController *keys = gtk_event_controller_key_new();
        gtk_event_controller_set_propagation_phase(keys, GTK_PHASE_CAPTURE);
        g_signal_connect(keys, "key-pressed", G_CALLBACK(shortcuts_key), NULL);
        gtk_widget_add_controller(GTK_WIDGET(shortcuts_window), keys);
    }
    gtk_window_present(shortcuts_window);
}

static gboolean key_pressed(GtkEventControllerKey *controller, guint key, guint code, GdkModifierType modifiers, gpointer data) {
    (void)controller; (void)code; (void)data;
    gboolean ctrl = (modifiers & GDK_CONTROL_MASK) != 0;
    GtkWidget *focus = gtk_window_get_focus(window);
    gboolean in_search = focus && (focus == search || gtk_widget_is_ancestor(focus, search));
    gboolean editing = focus && (GTK_IS_EDITABLE(focus) || gtk_widget_is_ancestor(focus, token_entry) || in_search);
    if (key == GDK_KEY_question && !(modifiers & (GDK_ALT_MASK | GDK_SUPER_MASK)) &&
        (ctrl || !editing || (in_search && !*yati_query()))) {
        event(YATI_HELP, 0, ""); return TRUE;
    }
    if (key == GDK_KEY_Escape) {
        if (g_strcmp0(gtk_stack_get_visible_child_name(GTK_STACK(main_stack)), "connection") == 0)
            yati_set_connection(0);
        else yati_hide();
        return TRUE;
    }
    if (ctrl && (key == GDK_KEY_q || key == GDK_KEY_Q)) { event(YATI_QUIT, 0, ""); return TRUE; }
    if (ctrl && (key == GDK_KEY_r || key == GDK_KEY_R)) { event(YATI_REFRESH, 0, ""); return TRUE; }
    if (timer_shortcut(key, modifiers)) return TRUE;
    if (ctrl && key == GDK_KEY_1) { yati_set_connection(0); return TRUE; }
    if (ctrl && key == GDK_KEY_2) { gtk_widget_set_visible(search, TRUE); gtk_stack_set_visible_child_name(GTK_STACK(main_stack), "history"); return TRUE; }
    if (ctrl && (key == GDK_KEY_l || key == GDK_KEY_L)) { gtk_widget_grab_focus(search); return TRUE; }
    if (key == GDK_KEY_F10) { gtk_menu_button_popup(GTK_MENU_BUTTON(app_menu)); return TRUE; }
    if (g_strcmp0(gtk_stack_get_visible_child_name(GTK_STACK(main_stack)), "connection") == 0)
        return FALSE;
    if (key == GDK_KEY_Return || key == GDK_KEY_KP_Enter) {
        if (ctrl) { event(YATI_START_NEW, 0, ""); return TRUE; }
        // Leave Enter on buttons and the searchable project picker to GTK.
        GtkWidget *focus = gtk_window_get_focus(window);
        if (focus != search && !gtk_widget_is_ancestor(focus, search) &&
            focus != recent_list && focus != history_list &&
            !gtk_widget_is_ancestor(focus, recent_list) && !gtk_widget_is_ancestor(focus, history_list))
            return FALSE;
        gboolean history = g_strcmp0(gtk_stack_get_visible_child_name(GTK_STACK(main_stack)), "history") == 0;
        GtkWidget *list = history ? history_list : recent_list;
        GtkListBoxRow *row = gtk_list_box_get_selected_row(GTK_LIST_BOX(list));
        if (row) row_activated(GTK_LIST_BOX(list), row, history ? GINT_TO_POINTER(1) : NULL);
        else if (!history) event(YATI_START_NEW, 0, "");
        return TRUE;
    }
    if ((key == GDK_KEY_Down || key == GDK_KEY_Up) &&
        (gtk_window_get_focus(window) == search || gtk_widget_is_ancestor(gtk_window_get_focus(window), search))) {
        gboolean history = g_strcmp0(gtk_stack_get_visible_child_name(GTK_STACK(main_stack)), "history") == 0;
        GtkListBox *list = GTK_LIST_BOX(history ? history_list : recent_list);
        GtkListBoxRow *selected = gtk_list_box_get_selected_row(list);
        int index = selected ? gtk_list_box_row_get_index(selected) : -1;
        GtkListBoxRow *row = gtk_list_box_get_row_at_index(list, MAX(0, index + (key == GDK_KEY_Down ? 1 : -1)));
        if (row) gtk_list_box_select_row(list, row);
        return TRUE;
    }
    return FALSE;
}

static void period_changed(GObject *object, GParamSpec *spec, gpointer data) {
    (void)spec; (void)data;
    event(YATI_PERIOD, (int)gtk_drop_down_get_selected(GTK_DROP_DOWN(object)), "");
}

static void page_changed(GObject *object, GParamSpec *spec, gpointer data) {
    (void)spec; (void)data;
    const char *name = gtk_stack_get_visible_child_name(GTK_STACK(object));
    event(YATI_PAGE, 0, name);
}

static void autostart_changed(GtkCheckButton *check, gpointer data) {
    (void)data;
    if (!rendering) event(YATI_AUTOSTART, gtk_check_button_get_active(check), "");
}

static void startup(GApplication *application, gpointer data) {
    (void)data;
    g_application_hold(application);
    window = GTK_WINDOW(adw_application_window_new(GTK_APPLICATION(app)));
    gtk_window_set_title(window, "Yati");
    gtk_window_set_default_size(window, 640, 690);
    gtk_widget_set_size_request(GTK_WIDGET(window), 420, 470);
    gtk_window_set_default_icon_name("io.github.kristof.yati");
    g_signal_connect(window, "close-request", G_CALLBACK(close_window), NULL);

    GtkWidget *root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    adw_application_window_set_content(ADW_APPLICATION_WINDOW(window), root);
    GtkWidget *header = adw_header_bar_new();
    adw_header_bar_set_title_widget(ADW_HEADER_BAR(header), adw_window_title_new("Yati", "Time, within reach"));
    gtk_box_append(GTK_BOX(root), header);
    GtkWidget *refresh = button(NULL, "view-refresh-symbolic", YATI_REFRESH);
    gtk_widget_set_tooltip_text(refresh, "Refresh from Toggl (Ctrl+R)");
    adw_header_bar_pack_start(ADW_HEADER_BAR(header), refresh);
    GtkWidget *menu = gtk_menu_button_new();
    app_menu = menu;
    gtk_menu_button_set_icon_name(GTK_MENU_BUTTON(menu), "open-menu-symbolic");
    gtk_widget_set_tooltip_text(menu, "Yati menu");
    GtkWidget *popover = gtk_popover_new();
    GtkWidget *menu_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_set_margin_start(menu_box, 8); gtk_widget_set_margin_end(menu_box, 8);
    gtk_widget_set_margin_top(menu_box, 8); gtk_widget_set_margin_bottom(menu_box, 8);
    GtkWidget *connection_button = button("Connection", "network-server-symbolic", YATI_CONNECTION);
    gtk_box_append(GTK_BOX(menu_box), connection_button);
    gtk_box_append(GTK_BOX(menu_box), button("Set global shortcut…", "preferences-desktop-keyboard-shortcuts-symbolic", YATI_SHORTCUT));
    gtk_box_append(GTK_BOX(menu_box), button("Keyboard shortcuts", "help-browser-symbolic", YATI_HELP));
    autostart_check = gtk_check_button_new_with_label("Start at login");
    g_signal_connect(autostart_check, "toggled", G_CALLBACK(autostart_changed), NULL);
    gtk_box_append(GTK_BOX(menu_box), autostart_check);
    gtk_box_append(GTK_BOX(menu_box), gtk_separator_new(GTK_ORIENTATION_HORIZONTAL));
    gtk_box_append(GTK_BOX(menu_box), button("Quit Yati", "application-exit-symbolic", YATI_QUIT));
    gtk_popover_set_child(GTK_POPOVER(popover), menu_box);
    gtk_menu_button_set_popover(GTK_MENU_BUTTON(menu), popover);
    adw_header_bar_pack_end(ADW_HEADER_BAR(header), menu);

    toasts = ADW_TOAST_OVERLAY(adw_toast_overlay_new());
    gtk_widget_set_vexpand(GTK_WIDGET(toasts), TRUE);
    gtk_box_append(GTK_BOX(root), GTK_WIDGET(toasts));
    GtkWidget *content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 16);
    gtk_widget_set_margin_start(content, 24); gtk_widget_set_margin_end(content, 24);
    gtk_widget_set_margin_top(content, 12); gtk_widget_set_margin_bottom(content, 18);
    adw_toast_overlay_set_child(toasts, content);

    error_revealer = gtk_revealer_new();
    GtkWidget *error_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
    gtk_widget_add_css_class(error_box, "card");
    error_label = label("", "error");
    gtk_label_set_wrap(GTK_LABEL(error_label), TRUE);
    gtk_widget_set_hexpand(error_label, TRUE);
    gtk_widget_set_margin_start(error_label, 12); gtk_widget_set_margin_top(error_label, 10); gtk_widget_set_margin_bottom(error_label, 10);
    gtk_box_append(GTK_BOX(error_box), error_label);
    GtkWidget *retry = button("Retry", NULL, YATI_REFRESH);
    gtk_widget_set_margin_end(retry, 8); gtk_widget_set_valign(retry, GTK_ALIGN_CENTER);
    gtk_box_append(GTK_BOX(error_box), retry);
    gtk_revealer_set_child(GTK_REVEALER(error_revealer), error_box);
    gtk_box_append(GTK_BOX(content), error_revealer);

    GtkWidget *current_list = gtk_list_box_new();
    gtk_widget_add_css_class(current_list, "boxed-list");
    gtk_list_box_set_selection_mode(GTK_LIST_BOX(current_list), GTK_SELECTION_NONE);
    current_row = adw_action_row_new();
    adw_preferences_row_set_use_markup(ADW_PREFERENCES_ROW(current_row), FALSE);
    adw_action_row_set_title_lines(ADW_ACTION_ROW(current_row), 2);
    adw_action_row_set_subtitle_lines(ADW_ACTION_ROW(current_row), 1);
    adw_action_row_add_prefix(ADW_ACTION_ROW(current_row), gtk_image_new_from_icon_name("alarm-symbolic"));
    elapsed_label = gtk_label_new("00:00:00");
    gtk_widget_add_css_class(elapsed_label, "numeric");
    gtk_widget_add_css_class(elapsed_label, "title-2");
    adw_action_row_add_suffix(ADW_ACTION_ROW(current_row), elapsed_label);
    stop_button = button("Stop", "media-playback-stop-symbolic", YATI_STOP);
    gtk_widget_set_valign(stop_button, GTK_ALIGN_CENTER);
    gtk_widget_add_css_class(stop_button, "destructive-action");
    gtk_widget_set_tooltip_text(stop_button, "Stop timer (Ctrl+S)");
    adw_action_row_add_suffix(ADW_ACTION_ROW(current_row), stop_button);
    continue_button = button("Continue last", "media-playback-start-symbolic", YATI_CONTINUE);
    gtk_widget_set_tooltip_text(continue_button, "Continue the last stopped entry (Ctrl+Shift+C)");
    gtk_widget_set_valign(continue_button, GTK_ALIGN_CENTER);
    adw_action_row_add_suffix(ADW_ACTION_ROW(current_row), continue_button);
    gtk_list_box_append(GTK_LIST_BOX(current_list), current_row);
    gtk_box_append(GTK_BOX(content), current_list);

    GtkWidget *switcher = gtk_stack_switcher_new();
    gtk_widget_set_halign(switcher, GTK_ALIGN_CENTER);
    gtk_box_append(GTK_BOX(content), switcher);
    search = gtk_search_entry_new();
    g_object_set(search, "placeholder-text", "Find a recent task or describe a new one…", NULL);
    gtk_accessible_update_property(GTK_ACCESSIBLE(search), GTK_ACCESSIBLE_PROPERTY_LABEL, "Task description or search", -1);
    g_signal_connect(search, "changed", G_CALLBACK(search_changed), NULL);
    gtk_box_append(GTK_BOX(content), search);

    main_stack = gtk_stack_new();
    gtk_widget_set_vexpand(main_stack, TRUE);
    gtk_stack_set_transition_type(GTK_STACK(main_stack), GTK_STACK_TRANSITION_TYPE_CROSSFADE);
    gtk_stack_set_transition_duration(GTK_STACK(main_stack), 120);
    gtk_stack_switcher_set_stack(GTK_STACK_SWITCHER(switcher), GTK_STACK(main_stack));
    g_signal_connect(main_stack, "notify::visible-child-name", G_CALLBACK(page_changed), NULL);
    gtk_box_append(GTK_BOX(content), main_stack);

    GtkWidget *track = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
    GtkWidget *composer = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    project_names = gtk_string_list_new(NULL);
    project_dropdown = gtk_drop_down_new(G_LIST_MODEL(project_names), NULL);
    gtk_drop_down_set_enable_search(GTK_DROP_DOWN(project_dropdown), TRUE);
    gtk_widget_set_hexpand(project_dropdown, TRUE);
    gtk_accessible_update_property(GTK_ACCESSIBLE(project_dropdown), GTK_ACCESSIBLE_PROPERTY_LABEL, "Project for new entry", -1);
    gtk_box_append(GTK_BOX(composer), project_dropdown);
    start_button = button("Start new", "media-playback-start-symbolic", YATI_START_NEW);
    gtk_widget_add_css_class(start_button, "suggested-action");
    gtk_widget_set_tooltip_text(start_button, "Start the typed description with this project (Ctrl+Enter)");
    gtk_box_append(GTK_BOX(composer), start_button);
    gtk_box_append(GTK_BOX(track), composer);
    GtkWidget *recent_heading = label("Recent tasks", "heading");
    gtk_widget_set_margin_top(recent_heading, 8);
    gtk_box_append(GTK_BOX(track), recent_heading);
    recent_list = gtk_list_box_new();
    gtk_widget_add_css_class(recent_list, "boxed-list");
    gtk_list_box_set_selection_mode(GTK_LIST_BOX(recent_list), GTK_SELECTION_SINGLE);
    gtk_list_box_set_activate_on_single_click(GTK_LIST_BOX(recent_list), TRUE);
    g_signal_connect(recent_list, "row-activated", G_CALLBACK(row_activated), NULL);
    GtkWidget *recent_scroll = gtk_scrolled_window_new();
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(recent_scroll), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    // Reserve the scrollbar's full width so it cannot overlap row contents.
    gtk_scrolled_window_set_overlay_scrolling(GTK_SCROLLED_WINDOW(recent_scroll), FALSE);
    gtk_widget_set_vexpand(recent_scroll, TRUE);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(recent_scroll), recent_list);
    gtk_box_append(GTK_BOX(track), recent_scroll);
    GtkWidget *hint = label("↑↓ Select   ·   Enter Continue   ·   Ctrl+Enter Start new", "caption");
    gtk_widget_add_css_class(hint, "dim-label");
    gtk_label_set_wrap(GTK_LABEL(hint), TRUE);
    gtk_box_append(GTK_BOX(track), hint);
    gtk_stack_add_titled(GTK_STACK(main_stack), track, "track", "Quick entry");

    GtkWidget *history = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
    const char *periods[] = {"Today", "This week", "This month", NULL};
    GtkWidget *period = gtk_drop_down_new_from_strings(periods);
    g_signal_connect(period, "notify::selected", G_CALLBACK(period_changed), NULL);
    gtk_box_append(GTK_BOX(history), period);
    summary_label = label("", "heading");
    gtk_box_append(GTK_BOX(history), summary_label);
    history_list = gtk_list_box_new();
    gtk_widget_add_css_class(history_list, "boxed-list");
    gtk_list_box_set_selection_mode(GTK_LIST_BOX(history_list), GTK_SELECTION_SINGLE);
    gtk_list_box_set_activate_on_single_click(GTK_LIST_BOX(history_list), TRUE);
    g_signal_connect(history_list, "row-activated", G_CALLBACK(row_activated), GINT_TO_POINTER(1));
    GtkWidget *history_scroll = gtk_scrolled_window_new();
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(history_scroll), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_widget_set_vexpand(history_scroll, TRUE);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(history_scroll), history_list);
    gtk_box_append(GTK_BOX(history), history_scroll);
    gtk_stack_add_titled(GTK_STACK(main_stack), history, "history", "History");

    GtkWidget *connection = gtk_box_new(GTK_ORIENTATION_VERTICAL, 14);
    gtk_box_append(GTK_BOX(connection), label("Connect to Toggl", "title-2"));
    GtkWidget *explanation = label("Use the API token from your Toggl profile. Your existing yati config, environment, and pass setup also work.", NULL);
    gtk_label_set_wrap(GTK_LABEL(explanation), TRUE);
    gtk_box_append(GTK_BOX(connection), explanation);
    GtkWidget *link = gtk_link_button_new_with_label("https://track.toggl.com/profile", "Open Toggl profile");
    gtk_widget_set_halign(link, GTK_ALIGN_START);
    gtk_box_append(GTK_BOX(connection), link);
    token_entry = gtk_password_entry_new();
    gtk_password_entry_set_show_peek_icon(GTK_PASSWORD_ENTRY(token_entry), TRUE);
    g_object_set(token_entry, "placeholder-text", "Toggl API token", NULL);
    gtk_accessible_update_property(GTK_ACCESSIBLE(token_entry), GTK_ACCESSIBLE_PROPERTY_LABEL, "Toggl API token", -1);
    g_signal_connect(token_entry, "changed", G_CALLBACK(token_changed), NULL);
    gtk_box_append(GTK_BOX(connection), token_entry);
    remember_check = gtk_check_button_new_with_label("Save in my yati config (readable only by me)");
    gtk_box_append(GTK_BOX(connection), remember_check);
    connect_button = button("Connect", NULL, YATI_CONNECT);
    gtk_widget_add_css_class(connect_button, "suggested-action");
    gtk_widget_set_sensitive(connect_button, FALSE);
    gtk_box_append(GTK_BOX(connection), connect_button);
    gtk_box_append(GTK_BOX(connection), button("Back to quick entry", NULL, YATI_ACTIVATE));
    gtk_stack_add_named(GTK_STACK(main_stack), connection, "connection");

    GtkWidget *status = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    spinner = gtk_spinner_new();
    gtk_box_append(GTK_BOX(status), spinner);
    status_label = label("Connecting to Toggl…", "caption");
    gtk_widget_add_css_class(status_label, "dim-label");
    gtk_label_set_ellipsize(GTK_LABEL(status_label), PANGO_ELLIPSIZE_END);
    gtk_box_append(GTK_BOX(status), status_label);
    gtk_box_append(GTK_BOX(content), status);

    GtkEventController *keys = gtk_event_controller_key_new();
    gtk_event_controller_set_propagation_phase(keys, GTK_PHASE_CAPTURE);
    g_signal_connect(keys, "key-pressed", G_CALLBACK(key_pressed), NULL);
    gtk_widget_add_controller(GTK_WIDGET(window), keys);
    yati_set_current("No timer running", "Start a task below or continue your last entry.", "", 0, 0, 1);
    event(YATI_STARTUP, 0, "");
}

void yati_present(const char *token) {
    yati_set_connection(0);
    if (token && *token) gtk_window_set_startup_id(window, token);
    gtk_window_present(window);
    gtk_widget_grab_focus(search);
    if (!clock_source) clock_source = g_timeout_add_seconds(1, tick, NULL);
}

static void activate(GApplication *application, gpointer data) {
    (void)application; (void)data;
    yati_present(NULL);
    event(YATI_ACTIVATE, 0, "");
}

static int command_line(GApplication *application, GApplicationCommandLine *command, gpointer data) {
    (void)data;
    GVariantDict *options = g_application_command_line_get_options_dict(command);
    if (g_variant_dict_contains(options, "quit")) {
        g_application_quit(application);
    } else if (g_variant_dict_contains(options, "toggle") && gtk_widget_get_visible(GTK_WIDGET(window))) {
        yati_hide();
    } else if (!g_variant_dict_contains(options, "background")) {
        g_application_activate(application);
    }
    return 0;
}

int yati_run(int argc, char **argv) {
    app = adw_application_new("io.github.kristof.yati", G_APPLICATION_HANDLES_COMMAND_LINE);
    g_application_add_main_option(G_APPLICATION(app), "background", 0, 0, G_OPTION_ARG_NONE, "Start in the tray", NULL);
    g_application_add_main_option(G_APPLICATION(app), "toggle", 0, 0, G_OPTION_ARG_NONE, "Toggle the quick-entry window", NULL);
    g_application_add_main_option(G_APPLICATION(app), "quit", 0, 0, G_OPTION_ARG_NONE, "Quit the resident app", NULL);
    g_signal_connect(app, "startup", G_CALLBACK(startup), NULL);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    g_signal_connect(app, "command-line", G_CALLBACK(command_line), NULL);
    int code = g_application_run(G_APPLICATION(app), argc, argv);
    if (clock_source) { g_source_remove(clock_source); clock_source = 0; }
    g_object_unref(app);
    return code;
}

typedef struct { int action; char *text; } PostedEvent;
static gboolean deliver(gpointer data) {
    PostedEvent *posted = data;
    event(posted->action, 0, posted->text);
    return G_SOURCE_REMOVE;
}
static void free_posted(gpointer data) {
    PostedEvent *posted = data;
    g_free(posted->text); g_free(posted);
}
void yati_post(int action, const char *text) {
    PostedEvent *posted = g_new0(PostedEvent, 1);
    posted->action = action; posted->text = g_strdup(text ? text : "");
    g_idle_add_full(G_PRIORITY_DEFAULT, deliver, posted, free_posted);
}
void yati_quit(void) { g_application_quit(G_APPLICATION(app)); }

void yati_set_current(const char *title, const char *subtitle, const char *elapsed, int running, int continuable, int busy) {
    adw_preferences_row_set_title(ADW_PREFERENCES_ROW(current_row), title);
    adw_action_row_set_subtitle(ADW_ACTION_ROW(current_row), subtitle);
    gtk_label_set_text(GTK_LABEL(elapsed_label), elapsed);
    gtk_widget_set_visible(elapsed_label, running);
    gtk_widget_set_visible(stop_button, running);
    gtk_widget_set_sensitive(stop_button, !busy);
    gtk_widget_set_visible(continue_button, !running);
    gtk_widget_set_sensitive(continue_button, continuable && !busy);
}

void yati_set_status(const char *text, int error, int busy) {
    working = busy;
    gtk_spinner_set_spinning(GTK_SPINNER(spinner), busy);
    gtk_widget_set_visible(spinner, busy);
    gtk_revealer_set_reveal_child(GTK_REVEALER(error_revealer), error);
    gtk_label_set_text(GTK_LABEL(error ? error_label : status_label), text);
    gtk_widget_set_sensitive(start_button, connected && !busy && *yati_query());
    gtk_widget_set_sensitive(connect_button, !busy && *yati_token());
    gtk_widget_set_sensitive(project_dropdown, connected && !busy);
}
void yati_set_connection(int visible) {
    gtk_stack_set_visible_child_name(GTK_STACK(main_stack), visible ? "connection" : "track");
    gtk_widget_set_visible(search, !visible);
    if (visible) gtk_widget_grab_focus(token_entry);
}
void yati_set_connected(int value) { connected = value; }
void yati_set_autostart(int enabled) {
    rendering = TRUE;
    gtk_check_button_set_active(GTK_CHECK_BUTTON(autostart_check), enabled);
    rendering = FALSE;
}
void yati_set_query(const char *text) { gtk_editable_set_text(GTK_EDITABLE(search), text); }
const char *yati_query(void) { return gtk_editable_get_text(GTK_EDITABLE(search)); }
const char *yati_token(void) { return gtk_editable_get_text(GTK_EDITABLE(token_entry)); }
int yati_remember(void) { return gtk_check_button_get_active(GTK_CHECK_BUTTON(remember_check)); }
int yati_project(void) { return (int)gtk_drop_down_get_selected(GTK_DROP_DOWN(project_dropdown)); }
void yati_projects_begin(void) {
    gtk_string_list_splice(project_names, 0, g_list_model_get_n_items(G_LIST_MODEL(project_names)), NULL);
}
void yati_project_add(const char *name) { gtk_string_list_append(project_names, name); }
void yati_projects_end(int selected) { gtk_drop_down_set_selected(GTK_DROP_DOWN(project_dropdown), selected); }
void yati_list_begin(int history) {
    GtkWidget *list = history ? history_list : recent_list;
    GtkWidget *child;
    while ((child = gtk_widget_get_first_child(list))) gtk_list_box_remove(GTK_LIST_BOX(list), child);
}
void yati_list_add(int history, int index, const char *title, const char *subtitle, const char *detail) {
    GtkWidget *row = adw_action_row_new();
    adw_preferences_row_set_use_markup(ADW_PREFERENCES_ROW(row), FALSE);
    adw_preferences_row_set_title(ADW_PREFERENCES_ROW(row), title);
    adw_action_row_set_subtitle(ADW_ACTION_ROW(row), subtitle);
    adw_action_row_set_title_lines(ADW_ACTION_ROW(row), 1);
    adw_action_row_set_subtitle_lines(ADW_ACTION_ROW(row), 1);
    gtk_list_box_row_set_activatable(GTK_LIST_BOX_ROW(row), TRUE);
    GtkWidget *info = gtk_label_new(detail);
    gtk_widget_add_css_class(info, "caption"); gtk_widget_add_css_class(info, "dim-label");
    adw_action_row_add_suffix(ADW_ACTION_ROW(row), info);
    adw_action_row_add_suffix(ADW_ACTION_ROW(row), gtk_image_new_from_icon_name("media-playback-start-symbolic"));
    g_object_set_data(G_OBJECT(row), "entry-index", GINT_TO_POINTER(index));
    g_object_set_data(G_OBJECT(row), "entry-detail", info);
    gtk_list_box_append(GTK_LIST_BOX(history ? history_list : recent_list), row);
}
static GtkLabel *list_detail(int history, int index) {
    GtkListBoxRow *row = gtk_list_box_get_row_at_index(GTK_LIST_BOX(history ? history_list : recent_list), index);
    return row ? g_object_get_data(G_OBJECT(row), "entry-detail") : NULL;
}
void yati_list_update_detail(int history, int index, const char *detail) {
    GtkLabel *info = list_detail(history, index);
    if (info) gtk_label_set_text(info, detail);
}
const char *yati_list_detail(int history, int index) {
    GtkLabel *info = list_detail(history, index);
    return info ? gtk_label_get_text(info) : "";
}
void yati_list_end(int history, const char *empty) {
    GtkListBox *list = GTK_LIST_BOX(history ? history_list : recent_list);
    GtkWidget *placeholder = label(empty, "dim-label");
    gtk_label_set_wrap(GTK_LABEL(placeholder), TRUE);
    gtk_label_set_justify(GTK_LABEL(placeholder), GTK_JUSTIFY_CENTER);
    gtk_widget_set_margin_top(placeholder, 28); gtk_widget_set_margin_bottom(placeholder, 28);
    gtk_widget_set_margin_start(placeholder, 16); gtk_widget_set_margin_end(placeholder, 16);
    gtk_widget_set_halign(placeholder, GTK_ALIGN_CENTER);
    gtk_list_box_set_placeholder(list, placeholder);
    GtkListBoxRow *first = gtk_list_box_get_row_at_index(list, 0);
    if (first) gtk_list_box_select_row(list, first);
}
void yati_set_summary(const char *text) { gtk_label_set_text(GTK_LABEL(summary_label), text); }
void yati_toast(const char *text) { adw_toast_overlay_add_toast(toasts, adw_toast_new(text)); }
int yati_width(void) { return gtk_widget_get_width(GTK_WIDGET(window)); }
int yati_height(void) { return gtk_widget_get_height(GTK_WIDGET(window)); }
int yati_transitioning(void) { return gtk_stack_get_transition_running(GTK_STACK(main_stack)); }
void yati_clear_token(void) { gtk_editable_set_text(GTK_EDITABLE(token_entry), ""); }
void yati_set_icon(int running) {
    gtk_window_set_icon_name(window, running ? "io.github.kristof.yati-tracking" : "io.github.kristof.yati-idle");
}
int yati_help_visible(void) {
    return shortcuts_window && gtk_widget_get_mapped(GTK_WIDGET(shortcuts_window)) &&
        gtk_widget_get_width(GTK_WIDGET(shortcuts_window)) > 0;
}
