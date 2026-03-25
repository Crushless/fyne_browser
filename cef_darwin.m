//go:build cgo && darwin

#import <Cocoa/Cocoa.h>

#include "cef_darwin.h"

#include <stdlib.h>
#include <string.h>

#include "include/cef_api_hash.h"

typedef struct {
  cef_client_t cef;
  struct fynecef_browser_s* owner;
} fynecef_client_wrapper_t;

typedef struct {
  cef_display_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_display_wrapper_t;

typedef struct {
  cef_context_menu_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_context_wrapper_t;

typedef struct {
  cef_load_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_load_wrapper_t;

typedef struct {
  cef_render_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_render_wrapper_t;

typedef struct {
  cef_request_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_request_wrapper_t;

typedef struct {
  cef_resource_request_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_resource_wrapper_t;

typedef struct {
  cef_life_span_handler_t cef;
  struct fynecef_browser_s* owner;
} fynecef_life_span_wrapper_t;

struct fynecef_browser_s {
  int ref_count;
  uintptr_t go_handle;
  fynecef_client_wrapper_t client;
  fynecef_display_wrapper_t display;
  fynecef_context_wrapper_t context;
  fynecef_load_wrapper_t load;
  fynecef_render_wrapper_t render;
  fynecef_request_wrapper_t request;
  fynecef_resource_wrapper_t resource;
  fynecef_life_span_wrapper_t life_span;
  cef_window_handle_t parent_view;
  int x;
  int y;
  int width;
  int height;
  cef_browser_t* browser;
};

uintptr_t fynecef_window_content_view(uintptr_t window_handle) {
  NSWindow* window = (NSWindow*)window_handle;
  if (window == nil) {
    return 0;
  }
  return (uintptr_t)[window contentView];
}

static void fynecef_owner_add_ref(struct fynecef_browser_s* owner) {
  __sync_add_and_fetch(&owner->ref_count, 1);
}

static int fynecef_owner_release(struct fynecef_browser_s* owner) {
  int refs = __sync_sub_and_fetch(&owner->ref_count, 1);
  if (refs == 0) {
    if (owner->browser != NULL) {
      owner->browser->base.release((cef_base_ref_counted_t*)owner->browser);
      owner->browser = NULL;
    }
    free(owner);
    return 1;
  }
  return 0;
}

static int fynecef_owner_has_one_ref(struct fynecef_browser_s* owner) {
  return __sync_add_and_fetch(&owner->ref_count, 0) == 1;
}

static int fynecef_owner_has_at_least_one_ref(struct fynecef_browser_s* owner) {
  return __sync_add_and_fetch(&owner->ref_count, 0) >= 1;
}

#define DEFINE_REFCOUNTING(prefix, wrapper_type)                                 \
  static void CEF_CALLBACK prefix##_add_ref(cef_base_ref_counted_t* self) {      \
    wrapper_type* wrapper = (wrapper_type*)self;                                 \
    fynecef_owner_add_ref(wrapper->owner);                                        \
  }                                                                              \
  static int CEF_CALLBACK prefix##_release(cef_base_ref_counted_t* self) {       \
    wrapper_type* wrapper = (wrapper_type*)self;                                 \
    return fynecef_owner_release(wrapper->owner);                                 \
  }                                                                              \
  static int CEF_CALLBACK prefix##_has_one_ref(cef_base_ref_counted_t* self) {   \
    wrapper_type* wrapper = (wrapper_type*)self;                                 \
    return fynecef_owner_has_one_ref(wrapper->owner);                             \
  }                                                                              \
  static int CEF_CALLBACK prefix##_has_at_least_one_ref(                         \
      cef_base_ref_counted_t* self) {                                            \
    wrapper_type* wrapper = (wrapper_type*)self;                                 \
    return fynecef_owner_has_at_least_one_ref(wrapper->owner);                    \
  }

DEFINE_REFCOUNTING(fynecef_client, fynecef_client_wrapper_t)
DEFINE_REFCOUNTING(fynecef_display, fynecef_display_wrapper_t)
DEFINE_REFCOUNTING(fynecef_context, fynecef_context_wrapper_t)
DEFINE_REFCOUNTING(fynecef_load, fynecef_load_wrapper_t)
DEFINE_REFCOUNTING(fynecef_render, fynecef_render_wrapper_t)
DEFINE_REFCOUNTING(fynecef_request, fynecef_request_wrapper_t)
DEFINE_REFCOUNTING(fynecef_resource, fynecef_resource_wrapper_t)
DEFINE_REFCOUNTING(fynecef_life_span, fynecef_life_span_wrapper_t)

static void fynecef_init_handler_base(cef_base_ref_counted_t* base,
                                      size_t size,
                                      void (*add_ref)(cef_base_ref_counted_t*),
                                      int (*release)(cef_base_ref_counted_t*),
                                      int (*has_one_ref)(cef_base_ref_counted_t*),
                                      int (*has_at_least_one_ref)(
                                          cef_base_ref_counted_t*)) {
  memset(base, 0, size);
  base->size = size;
  base->add_ref = add_ref;
  base->release = release;
  base->has_one_ref = has_one_ref;
  base->has_at_least_one_ref = has_at_least_one_ref;
}

static void fynecef_string_from_utf8(const char* src, cef_string_t* dst) {
  if (src == NULL) {
    return;
  }
  cef_string_utf8_to_utf16(src, strlen(src), dst);
}

static void fynecef_clear_string(cef_string_t* s) {
  if (s != NULL) {
    cef_string_utf16_clear(s);
  }
}

static void fynecef_call_go_string(void (*fn)(uintptr_t, char*),
                                   uintptr_t handle,
                                   const cef_string_t* value) {
  cef_string_utf8_t utf8 = {};
  if (value != NULL && value->str != NULL && value->length > 0 &&
      cef_string_utf16_to_utf8(value->str, value->length, &utf8)) {
    fn(handle, utf8.str != NULL ? utf8.str : "");
    cef_string_utf8_clear(&utf8);
    return;
  }
  fn(handle, "");
}

static void fynecef_call_go_userfree(void (*fn)(uintptr_t, char*),
                                     uintptr_t handle,
                                     cef_string_userfree_t value) {
  cef_string_utf8_t utf8 = {};
  if (value != NULL && value->str != NULL && value->length > 0 &&
      cef_string_utf16_to_utf8(value->str, value->length, &utf8)) {
    fn(handle, utf8.str != NULL ? utf8.str : "");
    cef_string_utf8_clear(&utf8);
  } else {
    fn(handle, "");
  }
  if (value != NULL) {
    cef_string_userfree_free(value);
  }
}

static cef_display_handler_t* CEF_CALLBACK
fynecef_client_get_display_handler(cef_client_t* self) {
  fynecef_client_wrapper_t* wrapper = (fynecef_client_wrapper_t*)self;
  wrapper->owner->display.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->display);
  return &wrapper->owner->display.cef;
}

static cef_context_menu_handler_t* CEF_CALLBACK
fynecef_client_get_context_menu_handler(cef_client_t* self) {
  fynecef_client_wrapper_t* wrapper = (fynecef_client_wrapper_t*)self;
  wrapper->owner->context.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->context);
  return &wrapper->owner->context.cef;
}

static cef_load_handler_t* CEF_CALLBACK
fynecef_client_get_load_handler(cef_client_t* self) {
  fynecef_client_wrapper_t* wrapper = (fynecef_client_wrapper_t*)self;
  wrapper->owner->load.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->load);
  return &wrapper->owner->load.cef;
}

static cef_render_handler_t* CEF_CALLBACK
fynecef_client_get_render_handler(cef_client_t* self) {
  fynecef_client_wrapper_t* wrapper = (fynecef_client_wrapper_t*)self;
  wrapper->owner->render.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->render);
  return &wrapper->owner->render.cef;
}

static cef_request_handler_t* CEF_CALLBACK
fynecef_client_get_request_handler(cef_client_t* self) {
  fynecef_client_wrapper_t* wrapper = (fynecef_client_wrapper_t*)self;
  wrapper->owner->request.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->request);
  return &wrapper->owner->request.cef;
}

static cef_life_span_handler_t* CEF_CALLBACK
fynecef_client_get_life_span_handler(cef_client_t* self) {
  fynecef_client_wrapper_t* wrapper = (fynecef_client_wrapper_t*)self;
  wrapper->owner->life_span.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->life_span);
  return &wrapper->owner->life_span.cef;
}

static void CEF_CALLBACK fynecef_on_address_change(cef_display_handler_t* self,
                                                   cef_browser_t* browser,
                                                   cef_frame_t* frame,
                                                   const cef_string_t* url) {
  fynecef_display_wrapper_t* wrapper = (fynecef_display_wrapper_t*)self;
  if (frame != NULL && frame->is_main != NULL && !frame->is_main(frame)) {
    return;
  }
  fynecef_call_go_string(goCEFOnAddressChange, wrapper->owner->go_handle, url);
}

static void CEF_CALLBACK fynecef_on_title_change(cef_display_handler_t* self,
                                                 cef_browser_t* browser,
                                                 const cef_string_t* title) {
  fynecef_display_wrapper_t* wrapper = (fynecef_display_wrapper_t*)self;
  fynecef_call_go_string(goCEFOnTitleChange, wrapper->owner->go_handle, title);
}

static void CEF_CALLBACK
fynecef_on_loading_progress_change(cef_display_handler_t* self,
                                   cef_browser_t* browser,
                                   double progress) {
  fynecef_display_wrapper_t* wrapper = (fynecef_display_wrapper_t*)self;
  goCEFOnLoadProgress(wrapper->owner->go_handle, progress);
}

static int CEF_CALLBACK fynecef_on_cursor_change(
    cef_display_handler_t* self,
    cef_browser_t* browser,
    cef_cursor_handle_t cursor,
    cef_cursor_type_t type,
    const cef_cursor_info_t* custom_cursor_info) {
  fynecef_display_wrapper_t* wrapper = (fynecef_display_wrapper_t*)self;
  goCEFOnCursorChange(wrapper->owner->go_handle, (int)type);
  return 1;
}

static char* fynecef_dup_userfree_string(cef_string_userfree_t value) {
  cef_string_utf8_t utf8 = {};
  char* result = NULL;
  size_t len;

  if (value != NULL && value->str != NULL && value->length > 0 &&
      cef_string_utf16_to_utf8(value->str, value->length, &utf8) &&
      utf8.str != NULL) {
    len = strlen(utf8.str);
    result = (char*)calloc(len + 1, sizeof(char));
    if (result != NULL) {
      memcpy(result, utf8.str, len);
      result[len] = '\0';
    }
  }

  cef_string_utf8_clear(&utf8);
  if (value != NULL) {
    cef_string_userfree_free(value);
  }
  return result;
}

static void fynecef_free_menu_items(fynecef_menu_item_t* items, size_t count) {
  size_t i;
  if (items == NULL) {
    return;
  }
  for (i = 0; i < count; i++) {
    free(items[i].label);
    fynecef_free_menu_items(items[i].children, items[i].child_count);
  }
  free(items);
}

static int fynecef_copy_menu_model(cef_menu_model_t* model,
                                   fynecef_menu_item_t** out_items,
                                   size_t* out_count) {
  size_t count, i, visible_count = 0, target = 0;
  fynecef_menu_item_t* items = NULL;

  if (out_items == NULL || out_count == NULL || model == NULL ||
      model->get_count == NULL) {
    return 0;
  }

  count = model->get_count(model);
  for (i = 0; i < count; i++) {
    if (model->is_visible_at != NULL && !model->is_visible_at(model, i)) {
      continue;
    }
    visible_count++;
  }

  if (visible_count == 0) {
    *out_items = NULL;
    *out_count = 0;
    return 1;
  }

  items =
      (fynecef_menu_item_t*)calloc(visible_count, sizeof(fynecef_menu_item_t));
  if (items == NULL) {
    return 0;
  }

  for (i = 0; i < count; i++) {
    cef_menu_model_t* child;
    if (model->is_visible_at != NULL && !model->is_visible_at(model, i)) {
      continue;
    }

    items[target].type =
        model->get_type_at != NULL ? (int)model->get_type_at(model, i) : 0;
    items[target].command_id =
        model->get_command_id_at != NULL ? model->get_command_id_at(model, i)
                                         : -1;
    items[target].enabled =
        model->is_enabled_at != NULL ? model->is_enabled_at(model, i) : 1;
    items[target].checked =
        model->is_checked_at != NULL ? model->is_checked_at(model, i) : 0;
    items[target].label = model->get_label_at != NULL
                              ? fynecef_dup_userfree_string(
                                    model->get_label_at(model, i))
                              : NULL;

    child =
        model->get_sub_menu_at != NULL ? model->get_sub_menu_at(model, i) : NULL;
    if (child != NULL &&
        !fynecef_copy_menu_model(child, &items[target].children,
                                 &items[target].child_count)) {
      fynecef_free_menu_items(items, visible_count);
      return 0;
    }
    target++;
  }

  *out_items = items;
  *out_count = visible_count;
  return 1;
}

static fynecef_context_menu_t* fynecef_create_context_menu(
    cef_context_menu_params_t* params,
    cef_menu_model_t* model,
    cef_run_context_menu_callback_t* callback) {
  fynecef_context_menu_t* menu;

  if (params == NULL || model == NULL || callback == NULL) {
    return NULL;
  }

  menu = (fynecef_context_menu_t*)calloc(1, sizeof(fynecef_context_menu_t));
  if (menu == NULL) {
    return NULL;
  }

  menu->x = params->get_xcoord != NULL ? params->get_xcoord(params) : 0;
  menu->y = params->get_ycoord != NULL ? params->get_ycoord(params) : 0;
  menu->callback = callback;

  if (callback->base.add_ref != NULL) {
    callback->base.add_ref((cef_base_ref_counted_t*)callback);
  }

  if (!fynecef_copy_menu_model(model, &menu->items, &menu->item_count)) {
    if (callback->base.release != NULL) {
      callback->base.release((cef_base_ref_counted_t*)callback);
    }
    free(menu);
    return NULL;
  }

  return menu;
}

static void fynecef_destroy_context_menu(fynecef_context_menu_t* menu) {
  if (menu == NULL) {
    return;
  }
  fynecef_free_menu_items(menu->items, menu->item_count);
  if (menu->callback != NULL && menu->callback->base.release != NULL) {
    menu->callback->base.release((cef_base_ref_counted_t*)menu->callback);
  }
  free(menu);
}

static void CEF_CALLBACK fynecef_on_before_context_menu(
    cef_context_menu_handler_t* self,
    cef_browser_t* browser,
    cef_frame_t* frame,
    cef_context_menu_params_t* params,
    cef_menu_model_t* model) {}

static int CEF_CALLBACK fynecef_run_context_menu(
    cef_context_menu_handler_t* self,
    cef_browser_t* browser,
    cef_frame_t* frame,
    cef_context_menu_params_t* params,
    cef_menu_model_t* model,
    cef_run_context_menu_callback_t* callback) {
  fynecef_context_wrapper_t* wrapper = (fynecef_context_wrapper_t*)self;
  fynecef_context_menu_t* menu =
      fynecef_create_context_menu(params, model, callback);
  if (menu == NULL) {
    return 0;
  }
  goCEFOnContextMenu(wrapper->owner->go_handle, menu);
  return 1;
}

static void CEF_CALLBACK
fynecef_on_loading_state_change(cef_load_handler_t* self,
                                cef_browser_t* browser,
                                int is_loading,
                                int can_go_back,
                                int can_go_forward) {
  fynecef_load_wrapper_t* wrapper = (fynecef_load_wrapper_t*)self;
  goCEFOnLoadingStateChange(wrapper->owner->go_handle, is_loading, can_go_back,
                            can_go_forward);
}

static void CEF_CALLBACK fynecef_on_load_error(cef_load_handler_t* self,
                                               cef_browser_t* browser,
                                               cef_frame_t* frame,
                                               cef_errorcode_t error_code,
                                               const cef_string_t* error_text,
                                               const cef_string_t* failed_url) {
  fynecef_load_wrapper_t* wrapper = (fynecef_load_wrapper_t*)self;
  cef_string_utf8_t error_text_utf8 = {};
  cef_string_utf8_t failed_url_utf8 = {};

  if (error_text != NULL && error_text->str != NULL && error_text->length > 0) {
    cef_string_utf16_to_utf8(error_text->str, error_text->length,
                             &error_text_utf8);
  }
  if (failed_url != NULL && failed_url->str != NULL && failed_url->length > 0) {
    cef_string_utf16_to_utf8(failed_url->str, failed_url->length,
                             &failed_url_utf8);
  }

  goCEFOnLoadError(wrapper->owner->go_handle, (int)error_code,
                   error_text_utf8.str != NULL ? error_text_utf8.str : "",
                   failed_url_utf8.str != NULL ? failed_url_utf8.str : "");

  cef_string_utf8_clear(&error_text_utf8);
  cef_string_utf8_clear(&failed_url_utf8);
}

static int CEF_CALLBACK fynecef_get_root_screen_rect(
    cef_render_handler_t* self,
    cef_browser_t* browser,
    cef_rect_t* rect) {
  fynecef_render_wrapper_t* wrapper = (fynecef_render_wrapper_t*)self;
  if (rect == NULL) {
    return 0;
  }
  rect->x = wrapper->owner->x;
  rect->y = wrapper->owner->y;
  rect->width = wrapper->owner->width > 0 ? wrapper->owner->width : 1;
  rect->height = wrapper->owner->height > 0 ? wrapper->owner->height : 1;
  return 1;
}

static void CEF_CALLBACK fynecef_get_view_rect(cef_render_handler_t* self,
                                               cef_browser_t* browser,
                                               cef_rect_t* rect) {
  fynecef_render_wrapper_t* wrapper = (fynecef_render_wrapper_t*)self;
  if (rect == NULL) {
    return;
  }
  rect->x = 0;
  rect->y = 0;
  rect->width = wrapper->owner->width > 0 ? wrapper->owner->width : 1;
  rect->height = wrapper->owner->height > 0 ? wrapper->owner->height : 1;
}

static int CEF_CALLBACK fynecef_get_screen_point(cef_render_handler_t* self,
                                                 cef_browser_t* browser,
                                                 int viewX,
                                                 int viewY,
                                                 int* screenX,
                                                 int* screenY) {
  fynecef_render_wrapper_t* wrapper = (fynecef_render_wrapper_t*)self;
  if (screenX == NULL || screenY == NULL) {
    return 0;
  }
  *screenX = wrapper->owner->x + viewX;
  *screenY = wrapper->owner->y + viewY;
  return 1;
}

static int CEF_CALLBACK fynecef_get_screen_info(
    cef_render_handler_t* self,
    cef_browser_t* browser,
    cef_screen_info_t* screen_info) {
  fynecef_render_wrapper_t* wrapper = (fynecef_render_wrapper_t*)self;
  if (screen_info == NULL) {
    return 0;
  }
  memset(screen_info, 0, sizeof(cef_screen_info_t));
  screen_info->size = sizeof(cef_screen_info_t);
  screen_info->device_scale_factor = 1.0f;
  screen_info->depth = 24;
  screen_info->depth_per_component = 8;
  screen_info->is_monochrome = 0;
  screen_info->rect.x = wrapper->owner->x;
  screen_info->rect.y = wrapper->owner->y;
  screen_info->rect.width = wrapper->owner->width > 0 ? wrapper->owner->width : 1;
  screen_info->rect.height = wrapper->owner->height > 0 ? wrapper->owner->height : 1;
  screen_info->available_rect = screen_info->rect;
  return 1;
}

static void CEF_CALLBACK fynecef_on_paint(cef_render_handler_t* self,
                                          cef_browser_t* browser,
                                          cef_paint_element_type_t type,
                                          size_t dirtyRectsCount,
                                          cef_rect_t const* dirtyRects,
                                          const void* buffer,
                                          int width,
                                          int height) {
  fynecef_render_wrapper_t* wrapper = (fynecef_render_wrapper_t*)self;
  if (type != PET_VIEW || buffer == NULL || width <= 0 || height <= 0) {
    return;
  }

  goCEFOnFrame(wrapper->owner->go_handle, (void*)buffer, width, height,
               width * 4, dirtyRectsCount, (cef_rect_t*)dirtyRects);
}

static void CEF_CALLBACK fynecef_on_accelerated_paint(
    cef_render_handler_t* self,
    cef_browser_t* browser,
    cef_paint_element_type_t type,
    size_t dirtyRectsCount,
    cef_rect_t const* dirtyRects,
    const cef_accelerated_paint_info_t* info) {
}

static cef_resource_request_handler_t* CEF_CALLBACK
fynecef_get_resource_request_handler(cef_request_handler_t* self,
                                     cef_browser_t* browser,
                                     cef_frame_t* frame,
                                     cef_request_t* request,
                                     int is_navigation,
                                     int is_download,
                                     const cef_string_t* request_initiator,
                                     int* disable_default_handling) {
  fynecef_request_wrapper_t* wrapper = (fynecef_request_wrapper_t*)self;
  wrapper->owner->resource.cef.base.add_ref(
      (cef_base_ref_counted_t*)&wrapper->owner->resource);
  return &wrapper->owner->resource.cef;
}

static void CEF_CALLBACK fynecef_on_render_view_ready(
    cef_request_handler_t* self,
    cef_browser_t* browser) {}

static void CEF_CALLBACK fynecef_on_render_process_terminated(
    cef_request_handler_t* self,
    cef_browser_t* browser,
    cef_termination_status_t status,
    int error_code,
    const cef_string_t* error_string) {}

static cef_return_value_t CEF_CALLBACK
fynecef_on_before_resource_load(cef_resource_request_handler_t* self,
                                cef_browser_t* browser,
                                cef_frame_t* frame,
                                cef_request_t* request,
                                cef_callback_t* callback) {
  fynecef_resource_wrapper_t* wrapper = (fynecef_resource_wrapper_t*)self;
  cef_string_userfree_t url = NULL;
  cef_string_userfree_t method = NULL;
  cef_string_utf8_t url_utf8 = {};
  cef_string_utf8_t method_utf8 = {};
  cef_string_utf8_t initiator_utf8 = {};
  int decision = 0;
  int resource_type = 0;
  int is_navigation = 0;

  if (request != NULL) {
    if (request->get_url != NULL) {
      url = request->get_url(request);
    }
    if (request->get_method != NULL) {
      method = request->get_method(request);
    }
    if (request->get_resource_type != NULL) {
      resource_type = (int)request->get_resource_type(request);
    }
  }
  if (frame != NULL && frame->is_main != NULL && frame->is_main(frame)) {
    is_navigation = 1;
  }
  if (request != NULL && url != NULL && url->str != NULL) {
    cef_string_utf16_to_utf8(url->str, url->length, &url_utf8);
  }
  if (request != NULL && method != NULL && method->str != NULL) {
    cef_string_utf16_to_utf8(method->str, method->length, &method_utf8);
  }
  decision = goCEFOnBeforeResourceLoad(
      wrapper->owner->go_handle, url_utf8.str != NULL ? url_utf8.str : "",
      method_utf8.str != NULL ? method_utf8.str : "",
      initiator_utf8.str != NULL ? initiator_utf8.str : "", resource_type,
      is_navigation);

  cef_string_utf8_clear(&url_utf8);
  cef_string_utf8_clear(&method_utf8);
  cef_string_utf8_clear(&initiator_utf8);
  if (url != NULL) {
    cef_string_userfree_free(url);
  }
  if (method != NULL) {
    cef_string_userfree_free(method);
  }
  if (decision != 0) {
    return RV_CANCEL;
  }
  return RV_CONTINUE;
}

static int fynecef_string_equals_literal(const cef_string_t* value,
                                         const char* literal) {
  cef_string_utf8_t utf8 = {};
  int match = 0;

  if (value == NULL || literal == NULL || value->str == NULL ||
      value->length == 0) {
    return 0;
  }

  if (cef_string_utf16_to_utf8(value->str, value->length, &utf8) &&
      utf8.str != NULL && strcmp(utf8.str, literal) == 0) {
    match = 1;
  }

  cef_string_utf8_clear(&utf8);
  return match;
}

static int fynecef_redirect_popup_to_current_tab(cef_browser_t* browser,
                                                 const cef_string_t* target_url) {
  cef_frame_t* frame;

  if (browser == NULL || target_url == NULL || target_url->str == NULL ||
      target_url->length == 0) {
    return 0;
  }

  if (fynecef_string_equals_literal(target_url, "about:blank")) {
    return 0;
  }

  frame = browser->get_main_frame != NULL ? browser->get_main_frame(browser)
                                          : NULL;
  if (frame == NULL || frame->load_url == NULL) {
    return 0;
  }

  frame->load_url(frame, target_url);
  return 1;
}

static int CEF_CALLBACK fynecef_on_before_popup(
    cef_life_span_handler_t* self,
    cef_browser_t* browser,
    cef_frame_t* frame,
    int popup_id,
    const cef_string_t* target_url,
    const cef_string_t* target_frame_name,
    cef_window_open_disposition_t target_disposition,
    int user_gesture,
    const cef_popup_features_t* popup_features,
    cef_window_info_t* window_info,
    cef_client_t** client,
    cef_browser_settings_t* settings,
    cef_dictionary_value_t** extra_info,
    int* no_javascript_access) {
  switch (target_disposition) {
    case CEF_WOD_NEW_FOREGROUND_TAB:
    case CEF_WOD_NEW_BACKGROUND_TAB:
    case CEF_WOD_NEW_POPUP:
    case CEF_WOD_NEW_WINDOW:
    case CEF_WOD_OFF_THE_RECORD:
    case CEF_WOD_SWITCH_TO_TAB:
      if (fynecef_redirect_popup_to_current_tab(browser, target_url)) {
        return 1;
      }
      break;
    case CEF_WOD_SAVE_TO_DISK:
    case CEF_WOD_NEW_PICTURE_IN_PICTURE:
    case CEF_WOD_IGNORE_ACTION:
      break;
    case CEF_WOD_UNKNOWN:
    case CEF_WOD_CURRENT_TAB:
    case CEF_WOD_SINGLETON_TAB:
    case CEF_WOD_NUM_VALUES:
      if (fynecef_redirect_popup_to_current_tab(browser, target_url)) {
        return 1;
      }
      break;
  }
  return 1;
}

static void CEF_CALLBACK fynecef_on_before_close(cef_life_span_handler_t* self,
                                                 cef_browser_t* browser) {
  fynecef_life_span_wrapper_t* wrapper = (fynecef_life_span_wrapper_t*)self;
  if (wrapper->owner->browser != NULL) {
    wrapper->owner->browser->base.release(
        (cef_base_ref_counted_t*)wrapper->owner->browser);
    wrapper->owner->browser = NULL;
  }
  goCEFOnBeforeClose(wrapper->owner->go_handle);
}

static void fynecef_init_client(struct fynecef_browser_s* owner) {
  owner->client.owner = owner;
  fynecef_init_handler_base(
      &owner->client.cef.base, sizeof(cef_client_t), fynecef_client_add_ref,
      fynecef_client_release, fynecef_client_has_one_ref,
      fynecef_client_has_at_least_one_ref);
  owner->client.cef.get_display_handler = fynecef_client_get_display_handler;
  owner->client.cef.get_context_menu_handler =
      fynecef_client_get_context_menu_handler;
  owner->client.cef.get_load_handler = fynecef_client_get_load_handler;
  owner->client.cef.get_render_handler = fynecef_client_get_render_handler;
  owner->client.cef.get_request_handler = fynecef_client_get_request_handler;
  owner->client.cef.get_life_span_handler = fynecef_client_get_life_span_handler;
}

static void fynecef_init_context_handler(struct fynecef_browser_s* owner) {
  owner->context.owner = owner;
  fynecef_init_handler_base(
      &owner->context.cef.base, sizeof(cef_context_menu_handler_t),
      fynecef_context_add_ref, fynecef_context_release,
      fynecef_context_has_one_ref, fynecef_context_has_at_least_one_ref);
  owner->context.cef.on_before_context_menu = fynecef_on_before_context_menu;
  owner->context.cef.run_context_menu = fynecef_run_context_menu;
}

static void fynecef_init_display_handler(struct fynecef_browser_s* owner) {
  owner->display.owner = owner;
  fynecef_init_handler_base(
      &owner->display.cef.base, sizeof(cef_display_handler_t),
      fynecef_display_add_ref, fynecef_display_release,
      fynecef_display_has_one_ref, fynecef_display_has_at_least_one_ref);
  owner->display.cef.on_address_change = fynecef_on_address_change;
  owner->display.cef.on_title_change = fynecef_on_title_change;
  owner->display.cef.on_loading_progress_change =
      fynecef_on_loading_progress_change;
  owner->display.cef.on_cursor_change = fynecef_on_cursor_change;
}

static void fynecef_init_load_handler(struct fynecef_browser_s* owner) {
  owner->load.owner = owner;
  fynecef_init_handler_base(
      &owner->load.cef.base, sizeof(cef_load_handler_t), fynecef_load_add_ref,
      fynecef_load_release, fynecef_load_has_one_ref,
      fynecef_load_has_at_least_one_ref);
  owner->load.cef.on_loading_state_change = fynecef_on_loading_state_change;
  owner->load.cef.on_load_error = fynecef_on_load_error;
}

static void fynecef_init_render_handler(struct fynecef_browser_s* owner) {
  owner->render.owner = owner;
  fynecef_init_handler_base(
      &owner->render.cef.base, sizeof(cef_render_handler_t),
      fynecef_render_add_ref, fynecef_render_release,
      fynecef_render_has_one_ref, fynecef_render_has_at_least_one_ref);
  owner->render.cef.get_root_screen_rect = fynecef_get_root_screen_rect;
  owner->render.cef.get_view_rect = fynecef_get_view_rect;
  owner->render.cef.get_screen_point = fynecef_get_screen_point;
  owner->render.cef.get_screen_info = fynecef_get_screen_info;
  owner->render.cef.on_paint = fynecef_on_paint;
  owner->render.cef.on_accelerated_paint = fynecef_on_accelerated_paint;
}

static void fynecef_init_request_handler(struct fynecef_browser_s* owner) {
  owner->request.owner = owner;
  fynecef_init_handler_base(
      &owner->request.cef.base, sizeof(cef_request_handler_t),
      fynecef_request_add_ref, fynecef_request_release,
      fynecef_request_has_one_ref, fynecef_request_has_at_least_one_ref);
  owner->request.cef.get_resource_request_handler =
      fynecef_get_resource_request_handler;
  owner->request.cef.on_render_view_ready = fynecef_on_render_view_ready;
  owner->request.cef.on_render_process_terminated =
      fynecef_on_render_process_terminated;
}

static void fynecef_init_resource_handler(struct fynecef_browser_s* owner) {
  owner->resource.owner = owner;
  fynecef_init_handler_base(
      &owner->resource.cef.base, sizeof(cef_resource_request_handler_t),
      fynecef_resource_add_ref, fynecef_resource_release,
      fynecef_resource_has_one_ref, fynecef_resource_has_at_least_one_ref);
  owner->resource.cef.on_before_resource_load =
      fynecef_on_before_resource_load;
}

static void fynecef_init_life_span_handler(struct fynecef_browser_s* owner) {
  owner->life_span.owner = owner;
  fynecef_init_handler_base(
      &owner->life_span.cef.base, sizeof(cef_life_span_handler_t),
      fynecef_life_span_add_ref, fynecef_life_span_release,
      fynecef_life_span_has_one_ref, fynecef_life_span_has_at_least_one_ref);
  owner->life_span.cef.on_before_popup = fynecef_on_before_popup;
  owner->life_span.cef.on_before_close = fynecef_on_before_close;
}

static int fynecef_configure_api_hash(void) {
  const char* api_hash = cef_api_hash(CEF_API_VERSION, 0);
  if (api_hash == NULL || strcmp(api_hash, CEF_API_HASH_PLATFORM) != 0) {
    return 0;
  }
  return 1;
}

int fynecef_execute_process(int argc, char** argv) {
  cef_main_args_t args = {.argc = argc, .argv = argv};
  if (!fynecef_configure_api_hash()) {
    return -1;
  }
  return cef_execute_process(&args, NULL, NULL);
}

int fynecef_initialize(int argc,
                       char** argv,
                       const char* subprocess_path,
                       const char* framework_dir,
                       const char* resources_dir,
                       const char* cache_path) {
  cef_main_args_t args = {.argc = argc, .argv = argv};
  cef_settings_t settings = {};

  if (!fynecef_configure_api_hash()) {
    return 0;
  }

  settings.size = sizeof(cef_settings_t);
  settings.no_sandbox = 1;
  settings.windowless_rendering_enabled = 1;
  settings.background_color = (cef_color_t)0xFFFFFFFFu;

  fynecef_string_from_utf8(subprocess_path, &settings.browser_subprocess_path);
  fynecef_string_from_utf8(framework_dir, &settings.framework_dir_path);
  fynecef_string_from_utf8(resources_dir, &settings.resources_dir_path);
  fynecef_string_from_utf8(cache_path, &settings.cache_path);
  fynecef_string_from_utf8(cache_path, &settings.root_cache_path);

  int ok = cef_initialize(&args, &settings, NULL, NULL);

  fynecef_clear_string(&settings.browser_subprocess_path);
  fynecef_clear_string(&settings.framework_dir_path);
  fynecef_clear_string(&settings.resources_dir_path);
  fynecef_clear_string(&settings.cache_path);
  fynecef_clear_string(&settings.root_cache_path);

  return ok;
}

void fynecef_shutdown(void) {
  cef_shutdown();
}

void fynecef_do_message_loop_work(void) {
  cef_do_message_loop_work();
}

fynecef_browser_t* fynecef_browser_create(uintptr_t go_handle,
                                          uintptr_t parent_view,
                                          int x,
                                          int y,
                                          int width,
                                          int height,
                                          const char* url) {
  struct fynecef_browser_s* owner =
      (struct fynecef_browser_s*)calloc(1, sizeof(struct fynecef_browser_s));
  cef_window_info_t window_info = {};
  cef_browser_settings_t settings = {};
  cef_string_t url_value = {};
  const char* target_url = url != NULL && url[0] != '\0' ? url : "about:blank";

  if (owner == NULL) {
    free(owner);
    return NULL;
  }

  owner->ref_count = 1;
  owner->go_handle = go_handle;
  owner->parent_view = (cef_window_handle_t)parent_view;
  owner->x = x;
  owner->y = y;
  owner->width = width > 0 ? width : 1;
  owner->height = height > 0 ? height : 1;

  fynecef_init_client(owner);
  fynecef_init_context_handler(owner);
  fynecef_init_display_handler(owner);
  fynecef_init_load_handler(owner);
  fynecef_init_render_handler(owner);
  fynecef_init_request_handler(owner);
  fynecef_init_resource_handler(owner);
  fynecef_init_life_span_handler(owner);

  window_info.size = sizeof(cef_window_info_t);
  window_info.parent_view = (cef_window_handle_t)parent_view;
  window_info.windowless_rendering_enabled = 1;
  window_info.bounds.x = x;
  window_info.bounds.y = y;
  window_info.bounds.width = owner->width;
  window_info.bounds.height = owner->height;
  window_info.runtime_style = CEF_RUNTIME_STYLE_ALLOY;

  settings.size = sizeof(cef_browser_settings_t);
  settings.windowless_frame_rate = 60;
  settings.background_color = (cef_color_t)0xFFFFFFFFu;
  fynecef_string_from_utf8(target_url, &url_value);

  owner->browser = cef_browser_host_create_browser_sync(
      &window_info, &owner->client.cef, &url_value, &settings, NULL, NULL);
  fynecef_clear_string(&url_value);

  if (owner->browser == NULL) {
    fynecef_owner_release(owner);
    return NULL;
  }

  cef_browser_host_t* host = owner->browser->get_host(owner->browser);
  if (host != NULL) {
    if (host->was_hidden != NULL) {
      host->was_hidden(host, 0);
    }
    if (host->notify_screen_info_changed != NULL) {
      host->notify_screen_info_changed(host);
    }
    if (host->was_resized != NULL) {
      host->was_resized(host);
    }
    if (host->invalidate != NULL) {
      host->invalidate(host, PET_VIEW);
    }
  }

  return owner;
}

void fynecef_browser_set_bounds(fynecef_browser_t* browser,
                                int x,
                                int y,
                                int width,
                                int height) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);

  if (host == NULL) {
    return;
  }

  browser->x = x;
  browser->y = y;
  browser->width = width > 0 ? width : 1;
  browser->height = height > 0 ? height : 1;

  if (host->notify_move_or_resize_started != NULL) {
    host->notify_move_or_resize_started(host);
  }
  if (host->was_resized != NULL) {
    host->was_resized(host);
  }
  if (host->notify_screen_info_changed != NULL) {
    host->notify_screen_info_changed(host);
  }
  if (host->invalidate != NULL) {
    host->invalidate(host, PET_VIEW);
  }
}

void fynecef_browser_set_windowless_frame_rate(fynecef_browser_t* browser,
                                               int frame_rate) {
  cef_browser_host_t* host;
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  host = browser->browser->get_host(browser->browser);
  if (host == NULL || host->set_windowless_frame_rate == NULL) {
    return;
  }
  if (frame_rate < 1) {
    frame_rate = 1;
  } else if (frame_rate > 60) {
    frame_rate = 60;
  }
  host->set_windowless_frame_rate(host, frame_rate);
}

void fynecef_browser_load_url(fynecef_browser_t* browser, const char* url) {
  if (browser == NULL || browser->browser == NULL || url == NULL) {
    return;
  }
  cef_frame_t* frame = browser->browser->get_main_frame(browser->browser);
  cef_string_t url_value = {};
  fynecef_string_from_utf8(url, &url_value);
  if (frame != NULL && frame->load_url != NULL) {
    frame->load_url(frame, &url_value);
  }
  fynecef_clear_string(&url_value);
}

void fynecef_browser_reload(fynecef_browser_t* browser) {
  if (browser == NULL || browser->browser == NULL || browser->browser->reload == NULL) {
    return;
  }
  browser->browser->reload(browser->browser);
}

void fynecef_browser_stop(fynecef_browser_t* browser) {
  if (browser == NULL || browser->browser == NULL || browser->browser->stop_load == NULL) {
    return;
  }
  browser->browser->stop_load(browser->browser);
}

void fynecef_browser_go_back(fynecef_browser_t* browser) {
  if (browser == NULL || browser->browser == NULL || browser->browser->go_back == NULL) {
    return;
  }
  browser->browser->go_back(browser->browser);
}

void fynecef_browser_go_forward(fynecef_browser_t* browser) {
  if (browser == NULL || browser->browser == NULL || browser->browser->go_forward == NULL) {
    return;
  }
  browser->browser->go_forward(browser->browser);
}

void fynecef_browser_set_focus(fynecef_browser_t* browser, int focus) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);
  if (host != NULL && host->set_focus != NULL) {
    host->set_focus(host, focus);
  }
}

static uint32_t fynecef_mouse_modifiers(uint32_t modifiers, int button) {
  switch (button) {
    case MBT_LEFT:
      return modifiers | EVENTFLAG_LEFT_MOUSE_BUTTON;
    case MBT_MIDDLE:
      return modifiers | EVENTFLAG_MIDDLE_MOUSE_BUTTON;
    case MBT_RIGHT:
      return modifiers | EVENTFLAG_RIGHT_MOUSE_BUTTON;
    default:
      return modifiers;
  }
}

void fynecef_browser_mouse_move(fynecef_browser_t* browser,
                                int x,
                                int y,
                                uint32_t modifiers,
                                int mouse_leave) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);
  cef_mouse_event_t event = {.x = x, .y = y, .modifiers = modifiers};
  if (host != NULL && host->send_mouse_move_event != NULL) {
    host->send_mouse_move_event(host, &event, mouse_leave);
  }
}

void fynecef_browser_mouse_click(fynecef_browser_t* browser,
                                 int x,
                                 int y,
                                 uint32_t modifiers,
                                 int button,
                                 int mouse_up,
                                 int click_count) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);
  cef_mouse_event_t event = {
      .x = x, .y = y, .modifiers = fynecef_mouse_modifiers(modifiers, button)};
  if (host != NULL && host->send_mouse_click_event != NULL) {
    host->send_mouse_click_event(host, &event, (cef_mouse_button_type_t)button,
                                 mouse_up, click_count);
  }
}

void fynecef_browser_mouse_wheel(fynecef_browser_t* browser,
                                 int x,
                                 int y,
                                 uint32_t modifiers,
                                 int delta_x,
                                 int delta_y) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);
  cef_mouse_event_t event = {.x = x, .y = y, .modifiers = modifiers};
  if (host != NULL && host->send_mouse_wheel_event != NULL) {
    host->send_mouse_wheel_event(host, &event, delta_x, delta_y);
  }
}

void fynecef_browser_key_event(fynecef_browser_t* browser,
                               int event_type,
                               uint32_t modifiers,
                               int windows_key_code,
                               int native_key_code,
                               uint16_t character,
                               uint16_t unmodified_character) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);
  cef_key_event_t event = {};
  event.size = sizeof(cef_key_event_t);
  event.type = (cef_key_event_type_t)event_type;
  event.modifiers = modifiers;
  event.windows_key_code = windows_key_code;
  event.native_key_code = native_key_code;
  event.character = character;
  event.unmodified_character = unmodified_character;
  if (host != NULL && host->send_key_event != NULL) {
    host->send_key_event(host, &event);
  }
}

void fynecef_browser_close(fynecef_browser_t* browser) {
  if (browser == NULL || browser->browser == NULL) {
    return;
  }
  cef_browser_host_t* host = browser->browser->get_host(browser->browser);
  if (host != NULL && host->close_browser != NULL) {
    host->close_browser(host, 1);
  }
}

void fynecef_copy_bgra_rect_to_rgba(uint8_t* dst,
                                    int dst_stride,
                                    const uint8_t* src,
                                    int src_stride,
                                    int x,
                                    int y,
                                    int width,
                                    int height) {
  int row;

  if (dst == NULL || src == NULL || dst_stride < width * 4 ||
      src_stride < (x + width) * 4 || x < 0 || y < 0 || width <= 0 ||
      height <= 0) {
    return;
  }

  src += (size_t)y * (size_t)src_stride + (size_t)x * 4u;
  for (row = 0; row < height; row++) {
    const uint8_t* src_row = src + (size_t)row * (size_t)src_stride;
    uint8_t* dst_row = dst + (size_t)row * (size_t)dst_stride;
    int i;

    for (i = 0; i < width * 4; i += 4) {
      dst_row[i] = src_row[i + 2];
      dst_row[i + 1] = src_row[i + 1];
      dst_row[i + 2] = src_row[i];
      dst_row[i + 3] = src_row[i + 3];
    }
  }
}

void fynecef_context_menu_continue(fynecef_context_menu_t* menu,
                                   int command_id,
                                   uint32_t event_flags) {
  if (menu == NULL) {
    return;
  }
  if (menu->callback != NULL && menu->callback->cont != NULL) {
    menu->callback->cont(menu->callback, command_id,
                         (cef_event_flags_t)event_flags);
  }
  fynecef_destroy_context_menu(menu);
}

void fynecef_context_menu_cancel(fynecef_context_menu_t* menu) {
  if (menu == NULL) {
    return;
  }
  if (menu->callback != NULL && menu->callback->cancel != NULL) {
    menu->callback->cancel(menu->callback);
  }
  fynecef_destroy_context_menu(menu);
}
