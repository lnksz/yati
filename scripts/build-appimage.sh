#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

mkdir -p tmp/appimage
output="$PWD/tmp/appimage"
build=$(mktemp -d "$output/build.XXXXXX")
trap 'rm -rf -- "$build"' EXIT
export TMPDIR="$build"
appdir="$build/Yati.AppDir"
app_id=io.github.kristof.yati

download() {
    curl --fail --silent --show-error --location --retry 3 "$1" -o "$build/$2"
    printf '%s  %s\n' "$3" "$build/$2" | sha256sum --check
}

download https://github.com/linuxdeploy/linuxdeploy/releases/download/1-alpha-20251107-1/linuxdeploy-x86_64.AppImage \
    linuxdeploy c20cd71e3a4e3b80c3483cef793cda3f4e990aca14014d23c544ca3ce1270b4d
download https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage \
    appimagetool ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0
download https://github.com/AppImage/type2-runtime/releases/download/20251108/runtime-x86_64 \
    runtime-x86_64 2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
chmod +x "$build/linuxdeploy" "$build/appimagetool"

mkdir -p "$appdir/usr/bin"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 "${GO:-go}" build -trimpath -ldflags='-s -w' -o "$appdir/usr/bin/yati" .

svg_loader="$(pkg-config --variable=gdk_pixbuf_moduledir gdk-pixbuf-2.0)/libpixbufloader-svg.so"
pixbuf_query=$(pkg-config --variable=gdk_pixbuf_query_loaders gdk-pixbuf-2.0)
gio_modules="$appdir/usr/lib/gio/modules"
dconf_module="$(pkg-config --variable=giomoduledir gio-2.0)/libdconfsettings.so"
install -Dm644 "$dconf_module" "$gio_modules/libdconfsettings.so"
desktop="contrib/appimage/$app_id.desktop"
desktop-file-validate "$desktop"
"$build/linuxdeploy" --appimage-extract-and-run --appdir "$appdir" \
    --desktop-file "$desktop" --custom-apprun contrib/appimage/AppRun \
    --icon-file internal/gui/assets/yati.svg --icon-filename "$app_id" \
    --library "$svg_loader" --deploy-deps-only "$gio_modules/libdconfsettings.so"
gio-querymodules "$gio_modules"

# dlopen expands $ORIGIN to the directory containing the bundled libgmodule.
cache="$appdir/usr/lib/yati-pixbuf-loaders.cache"
"$pixbuf_query" "$svg_loader" > "$cache"
# shellcheck disable=SC2016
sed -i 's|^".*/libpixbufloader-svg\.so"$|"$ORIGIN/libpixbufloader-svg.so"|' "$cache"
# shellcheck disable=SC2016
grep -Fqx '"$ORIGIN/libpixbufloader-svg.so"' "$cache"

share="$appdir/usr/share"
schemas="$share/glib-2.0/schemas"
mkdir -p "$schemas"
cp /usr/share/glib-2.0/schemas/org.gtk.gtk4.*.gschema.xml "$schemas/"
glib-compile-schemas --strict "$schemas"
# GdkPixbuf uses shared MIME data to recognize SVG icons.
install -Dm644 /usr/share/mime/packages/freedesktop.org.xml "$share/mime/packages/freedesktop.org.xml"
XDG_DATA_DIRS="$share" update-mime-database "$share/mime"
cp -a /usr/share/icons/Adwaita "$share/icons/"
cp /usr/share/icons/hicolor/index.theme "$share/icons/hicolor/"
icons="$share/icons/hicolor/scalable/apps"
install -m644 internal/gui/assets/yati.svg "$icons/$app_id-tracking.svg"
install -m644 internal/gui/assets/yati-idle.svg "$icons/$app_id-idle.svg"
install -Dm644 internal/gui/assets/NOTICE.md "$share/doc/yati/NOTICE.md"
for package in libgtk-4-common dconf-gsettings-backend shared-mime-info adwaita-icon-theme hicolor-icon-theme; do
    install -Dm644 "/usr/share/doc/$package/copyright" "$share/doc/$package/copyright"
done

ARCH=x86_64 "$build/appimagetool" --appimage-extract-and-run --no-appstream \
    --runtime-file "$build/runtime-x86_64" "$appdir" "$build/Yati-x86_64.AppImage"
mv -f -- "$build/Yati-x86_64.AppImage" "$output/"
