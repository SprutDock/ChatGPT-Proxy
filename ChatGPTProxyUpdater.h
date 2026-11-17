#import <Foundation/Foundation.h>

@interface ChatGPTProxyUpdater : NSObject
- (void)checkForUpdatesWithCompletion:(void (^)(void))completion;
@end
