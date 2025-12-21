#include "../common/RaylibSim.h"
#include "../common/Shapes.h"
#include "../common/Animator.h"

class MySim : public RaylibSim {
    Animator anim;
public:
    float speed = 1.0f; 
    float size = 2.0f;
    float myVal = 0.0f; 

    void update(float dt) override {
        anim.update(dt);
        myVal += dt * speed; 
    }

    void draw() override {
        beginDraw();
        Draw::Grid(10.0f, 1.0f);
        
        // Butterfly Curve
        Draw::Parametric(
            [this](float t){ return sin(t) * (exp(cos(t)) - 2*cos(4*t) - pow(sin(t/12.0f), 5)) * (size * 0.5f); },
            [this](float t){ return cos(t) * (exp(cos(t)) - 2*cos(4*t) - pow(sin(t/12.0f), 5)) * (size * 0.5f); },
            0, 50.0f, 
            ORANGE
        );

        endDraw();
    }
};

MySim* simInstance = nullptr;

void UpdateDrawFrame() { 
    if (simInstance) { 
        simInstance->update(GetFrameTime()); 
        simInstance->draw(); 
    } 
}

// FIX: Helper function
MySim* getSimInstance() {
    return simInstance;
}

int main() {
    SetConfigFlags(FLAG_MSAA_4X_HINT | FLAG_WINDOW_HIGHDPI);
    InitWindow(800, 600, "MySim");
    simInstance = new MySim();
    emscripten_set_main_loop(UpdateDrawFrame, 0, 1);
    return 0;
}

EMSCRIPTEN_BINDINGS(my_sim_module) {
    class_<MySim>("Simulation")
        .function("initHelper", &MySim::initHelper)
        .property("speed", &MySim::speed)
        .property("size", &MySim::size);
        
    // FIX: Bind the helper function
    function("getInstance", &getSimInstance, allow_raw_pointers());
}