//go:build cgo && darwin

#import "webkit_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

struct fynecef_browser_s {
  uintptr_t go_handle;
  NSWindow* window;
  NSView* content_view;
  WKWebView* web_view;
  id delegate;
  int x;
  int y;
  int width;
  int height;
  int closed;
};

static void* fynecef_title_context = &fynecef_title_context;
static void* fynecef_url_context = &fynecef_url_context;
static void* fynecef_progress_context = &fynecef_progress_context;

static void fynecef_sync_main(dispatch_block_t block) {
  if ([NSThread isMainThread]) {
    block();
    return;
  }
  dispatch_sync(dispatch_get_main_queue(), block);
}

static const char* fynecef_string_utf8(NSString* value) {
  if (value == nil) {
    return "";
  }
  const char* utf8 = [value UTF8String];
  return utf8 != NULL ? utf8 : "";
}

static NSString* fynecef_nsstring(const char* value) {
  if (value == NULL || value[0] == '\0') {
    return @"";
  }
  return [NSString stringWithUTF8String:value];
}

static NSURL* fynecef_parse_url(const char* value) {
  NSString* raw = fynecef_nsstring(value);
  if ([raw length] == 0) {
    return [NSURL URLWithString:@"about:blank"];
  }
  NSURL* url = [NSURL URLWithString:raw];
  if (url != nil) {
    return url;
  }
  return [NSURL URLWithString:@"about:blank"];
}

static void fynecef_emit_address(struct fynecef_browser_s* owner) {
  if (owner == NULL || owner->web_view == nil) {
    return;
  }
  NSString* value = owner->web_view.URL.absoluteString;
  goCEFOnAddressChange(owner->go_handle, (char*)fynecef_string_utf8(value));
}

static void fynecef_emit_title(struct fynecef_browser_s* owner) {
  if (owner == NULL || owner->web_view == nil) {
    return;
  }
  goCEFOnTitleChange(owner->go_handle, (char*)fynecef_string_utf8(owner->web_view.title));
}

static void fynecef_emit_progress(struct fynecef_browser_s* owner) {
  if (owner == NULL || owner->web_view == nil) {
    return;
  }
  goCEFOnLoadProgress(owner->go_handle, owner->web_view.estimatedProgress);
}

static void fynecef_emit_loading_state(struct fynecef_browser_s* owner, int is_loading) {
  if (owner == NULL || owner->web_view == nil) {
    return;
  }
  goCEFOnLoadingStateChange(owner->go_handle, is_loading,
                            owner->web_view.canGoBack ? 1 : 0,
                            owner->web_view.canGoForward ? 1 : 0);
}

static void fynecef_apply_bounds(struct fynecef_browser_s* owner) {
  if (owner == NULL || owner->content_view == nil || owner->web_view == nil) {
    return;
  }

  CGFloat width = owner->width > 0 ? owner->width : 1;
  CGFloat height = owner->height > 0 ? owner->height : 1;
  CGFloat content_height = owner->content_view.bounds.size.height;
  CGFloat origin_x = owner->x;
  CGFloat origin_y = content_height - owner->y - height;

  if (origin_y < 0) {
    origin_y = 0;
  }

  owner->web_view.frame = NSMakeRect(origin_x, origin_y, width, height);
  owner->web_view.hidden = owner->closed || owner->width <= 0 || owner->height <= 0;
}

@interface FyneCEFNavigationDelegate : NSObject <WKNavigationDelegate>
- (instancetype)initWithOwner:(struct fynecef_browser_s*)owner;
@end

@implementation FyneCEFNavigationDelegate {
  struct fynecef_browser_s* _owner;
}

- (instancetype)initWithOwner:(struct fynecef_browser_s*)owner {
  self = [super init];
  if (self != nil) {
    _owner = owner;
  }
  return self;
}

- (void)observeValueForKeyPath:(NSString*)keyPath
                      ofObject:(id)object
                        change:(NSDictionary<NSKeyValueChangeKey, id>*)change
                       context:(void*)context {
  if (_owner == NULL || _owner->closed) {
    return;
  }

  if (context == fynecef_title_context) {
    fynecef_emit_title(_owner);
    return;
  }
  if (context == fynecef_url_context) {
    fynecef_emit_address(_owner);
    fynecef_emit_loading_state(_owner, _owner->web_view.loading ? 1 : 0);
    return;
  }
  if (context == fynecef_progress_context) {
    fynecef_emit_progress(_owner);
    fynecef_emit_loading_state(_owner, _owner->web_view.loading ? 1 : 0);
    return;
  }

  [super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
}

- (void)webView:(WKWebView*)webView
    didStartProvisionalNavigation:(WKNavigation*)navigation {
  if (_owner == NULL || _owner->closed) {
    return;
  }
  fynecef_emit_address(_owner);
  fynecef_emit_progress(_owner);
  fynecef_emit_loading_state(_owner, 1);
}

- (void)webView:(WKWebView*)webView
    didCommitNavigation:(WKNavigation*)navigation {
  if (_owner == NULL || _owner->closed) {
    return;
  }
  fynecef_emit_address(_owner);
}

- (void)webView:(WKWebView*)webView
    didFinishNavigation:(WKNavigation*)navigation {
  if (_owner == NULL || _owner->closed) {
    return;
  }
  fynecef_emit_address(_owner);
  fynecef_emit_title(_owner);
  fynecef_emit_progress(_owner);
  fynecef_emit_loading_state(_owner, 0);
}

- (void)webView:(WKWebView*)webView
    didFailNavigation:(WKNavigation*)navigation
            withError:(NSError*)error {
  if (_owner == NULL || _owner->closed) {
    return;
  }
  goCEFOnLoadError(_owner->go_handle, (int)error.code,
                   (char*)fynecef_string_utf8(error.localizedDescription),
                   (char*)fynecef_string_utf8(webView.URL.absoluteString));
  fynecef_emit_loading_state(_owner, 0);
}

- (void)webView:(WKWebView*)webView
    didFailProvisionalNavigation:(WKNavigation*)navigation
                       withError:(NSError*)error {
  if (_owner == NULL || _owner->closed) {
    return;
  }
  goCEFOnLoadError(_owner->go_handle, (int)error.code,
                   (char*)fynecef_string_utf8(error.localizedDescription),
                   (char*)fynecef_string_utf8(webView.URL.absoluteString));
  fynecef_emit_loading_state(_owner, 0);
}

- (void)webView:(WKWebView*)webView
    decidePolicyForNavigationAction:(WKNavigationAction*)navigationAction
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
  if (_owner == NULL || _owner->closed) {
    decisionHandler(WKNavigationActionPolicyCancel);
    return;
  }

  NSURLRequest* request = navigationAction.request;
  NSString* url = request.URL.absoluteString;
  NSString* method = request.HTTPMethod;
  NSString* initiator = request.mainDocumentURL.absoluteString;
  BOOL isMainFrame = navigationAction.targetFrame == nil || navigationAction.targetFrame.mainFrame;

  int blocked = goCEFOnBeforeResourceLoad(
      _owner->go_handle,
      (char*)fynecef_string_utf8(url),
      (char*)fynecef_string_utf8(method),
      (char*)fynecef_string_utf8(initiator),
      0,
      isMainFrame ? 1 : 0);
  if (blocked != 0) {
    decisionHandler(WKNavigationActionPolicyCancel);
    return;
  }

  decisionHandler(WKNavigationActionPolicyAllow);
}

@end

fynecef_browser_t* fynecef_browser_create(uintptr_t go_handle,
                                          uintptr_t window_handle,
                                          int x,
                                          int y,
                                          int width,
                                          int height,
                                          const char* url) {
  __block fynecef_browser_t* owner = NULL;

  fynecef_sync_main(^{
    NSWindow* window = (NSWindow*)window_handle;
    NSView* content_view = window != nil ? window.contentView : nil;
    if (window == nil || content_view == nil) {
      return;
    }

    owner = (fynecef_browser_t*)calloc(1, sizeof(struct fynecef_browser_s));
    if (owner == NULL) {
      return;
    }

    owner->go_handle = go_handle;
    owner->window = [window retain];
    owner->content_view = [content_view retain];
    owner->x = x;
    owner->y = y;
    owner->width = width;
    owner->height = height;

    WKWebViewConfiguration* configuration = [[WKWebViewConfiguration alloc] init];
    WKWebView* web_view = [[WKWebView alloc] initWithFrame:NSZeroRect configuration:configuration];
    FyneCEFNavigationDelegate* delegate = [[FyneCEFNavigationDelegate alloc] initWithOwner:owner];
    [configuration release];

    owner->web_view = web_view;
    owner->delegate = delegate;

    web_view.navigationDelegate = delegate;
    [web_view addObserver:delegate
               forKeyPath:@"title"
                  options:NSKeyValueObservingOptionNew
                  context:fynecef_title_context];
    [web_view addObserver:delegate
               forKeyPath:@"URL"
                  options:NSKeyValueObservingOptionNew
                  context:fynecef_url_context];
    [web_view addObserver:delegate
               forKeyPath:@"estimatedProgress"
                  options:NSKeyValueObservingOptionNew
                  context:fynecef_progress_context];

    [content_view addSubview:web_view positioned:NSWindowAbove relativeTo:nil];
    fynecef_apply_bounds(owner);

    NSURL* ns_url = fynecef_parse_url(url);
    if (ns_url != nil) {
      NSURLRequest* request = [NSURLRequest requestWithURL:ns_url];
      [web_view loadRequest:request];
    }
  });

  return owner;
}

void fynecef_browser_set_bounds(fynecef_browser_t* browser,
                                int x,
                                int y,
                                int width,
                                int height) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed) {
      return;
    }
    browser->x = x;
    browser->y = y;
    browser->width = width;
    browser->height = height;
    fynecef_apply_bounds(browser);
  });
}

void fynecef_browser_load_url(fynecef_browser_t* browser, const char* url) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed || browser->web_view == nil) {
      return;
    }
    NSURL* ns_url = fynecef_parse_url(url);
    if (ns_url == nil) {
      return;
    }
    NSURLRequest* request = [NSURLRequest requestWithURL:ns_url];
    [browser->web_view loadRequest:request];
  });
}

void fynecef_browser_reload(fynecef_browser_t* browser) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed || browser->web_view == nil) {
      return;
    }
    [browser->web_view reload];
  });
}

void fynecef_browser_stop(fynecef_browser_t* browser) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed || browser->web_view == nil) {
      return;
    }
    [browser->web_view stopLoading];
  });
}

void fynecef_browser_go_back(fynecef_browser_t* browser) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed || browser->web_view == nil) {
      return;
    }
    if (browser->web_view.canGoBack) {
      [browser->web_view goBack];
    }
  });
}

void fynecef_browser_go_forward(fynecef_browser_t* browser) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed || browser->web_view == nil) {
      return;
    }
    if (browser->web_view.canGoForward) {
      [browser->web_view goForward];
    }
  });
}

void fynecef_browser_set_focus(fynecef_browser_t* browser, int focus) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed || browser->web_view == nil || browser->window == nil) {
      return;
    }
    if (focus) {
      [browser->window makeFirstResponder:browser->web_view];
      [[browser->window contentView] setNeedsDisplay:YES];
      return;
    }
    [browser->window makeFirstResponder:browser->content_view];
  });
}

void fynecef_browser_close(fynecef_browser_t* browser) {
  fynecef_sync_main(^{
    if (browser == NULL || browser->closed) {
      return;
    }

    browser->closed = 1;
    if (browser->web_view != nil && browser->delegate != nil) {
      @try {
        [browser->web_view removeObserver:browser->delegate forKeyPath:@"title" context:fynecef_title_context];
        [browser->web_view removeObserver:browser->delegate forKeyPath:@"URL" context:fynecef_url_context];
        [browser->web_view removeObserver:browser->delegate forKeyPath:@"estimatedProgress" context:fynecef_progress_context];
      } @catch (NSException* exception) {
      }
      browser->web_view.navigationDelegate = nil;
    }
    if (browser->web_view != nil) {
      [browser->web_view stopLoading];
      [browser->web_view removeFromSuperview];
      [browser->web_view release];
      browser->web_view = nil;
    }
    if (browser->delegate != nil) {
      [browser->delegate release];
      browser->delegate = nil;
    }
    if (browser->content_view != nil) {
      [browser->content_view release];
      browser->content_view = nil;
    }
    if (browser->window != nil) {
      [browser->window release];
      browser->window = nil;
    }
    goCEFOnBeforeClose(browser->go_handle);
    free(browser);
  });
}
