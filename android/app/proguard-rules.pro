# kotlinx.serialization
-keepattributes *Annotation*, InnerClasses, Signature, EnclosingMethod
-keepclassmembers @kotlinx.serialization.Serializable class ** { *** Companion; *; }
-keep,includedescriptorclasses class com.ryderdavid.nacre.**$$serializer { *; }
# anthropic-java ships its own consumer rules (META-INF/proguard); these only silence optional deps
-dontwarn com.fasterxml.jackson.**
-dontwarn com.anthropic.**
-dontwarn org.slf4j.**
-dontwarn java.beans.**
-dontwarn javax.annotation.**
-dontwarn kotlin.reflect.jvm.internal.**
# victools jsonschema-generator: only used by the SDK's class-derived schemas (not used here)
-dontwarn com.github.victools.jsonschema.**
-dontwarn java.lang.reflect.AnnotatedType
-dontwarn java.lang.reflect.AnnotatedParameterizedType

# ML Kit component discovery (GmsBarcodeScanning): MlKitInitProvider reflects on the
# ComponentRegistrars named in manifest meta-data. firebase-components' consumer rule keeps
# the classes but not their members, so R8 full mode strips the no-arg constructors and
# discovery fails, leaving GmsBarcodeScanning.getClient() to NPE on first scan.
-keep class * implements com.google.firebase.components.ComponentRegistrar { <init>(); }
